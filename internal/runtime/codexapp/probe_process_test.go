package codexapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func probeFixture(t *testing.T, root, script string) string {
	t.Helper()
	path := filepath.Join(root, "synthetic-codex")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func probeFixtureDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical.BytesDigest(data)
}

func TestProbeOutputOverflowCancelsWithoutGrowingDiagnostics(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	output := &probeOutput{limit: 64, cancel: cancel}
	var writers sync.WaitGroup
	for range 4 {
		writers.Go(func() {
			for range 32 {
				_, _ = output.Write([]byte(strings.Repeat("fixture", 32)))
				data, _ := output.snapshot()
				if len(data) > 64 {
					t.Error("output exceeded retained memory budget")
				}
			}
		})
	}
	writers.Wait()
	data, overflow := output.snapshot()
	if len(data) != 64 || !overflow || !errors.Is(context.Cause(ctx), ErrProbeOutputLimit) {
		t.Fatalf("overflow did not bound output and cancel: %d %v %v", len(data), overflow, context.Cause(ctx))
	}
}

func TestProbeCleanupCannotHideMissingReap(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	prior := errors.New("synthetic operation failure")
	stopFailure := errors.New("synthetic stop failure")
	proof, err := settleProbeExit(parent, func(stopping context.Context) (processscope.Proof, error) {
		deadline, ok := stopping.Deadline()
		if stopping.Err() != nil || !ok || time.Until(deadline) > probeStopTimeout {
			t.Fatal("cleanup inherited cancellation or has no bounded budget")
		}
		return processscope.Proof{}, stopFailure
	}, processscope.Proof{}, prior)
	if proof.Quiescent() || !errors.Is(err, prior) || !errors.Is(err, stopFailure) || !errors.Is(err, ErrProbeStopUnconfirmed) {
		t.Fatalf("stop failure was hidden: %+v %v", proof, err)
	}
	proof, err = settleProbeExit(parent, func(context.Context) (processscope.Proof, error) {
		return testsupport.ProcessScopeFixture(), nil
	}, processscope.Proof{}, prior)
	if !proof.Quiescent() || err != prior {
		t.Fatal("successful cleanup erased the original failure", proof, err)
	}
}

func TestCodexProbeRejectsOversizedOutput(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, lane := range []string{"version-stdout", "version-stderr", "command-stdout", "command-combined"} {
		t.Run(lane, func(t *testing.T) {
			root := t.TempDir()
			write := "os.write(1, b'x' * (4 << 20))"
			if lane == "version-stderr" {
				write = "os.write(2, b'x' * (4 << 20))"
			} else if lane == "command-combined" {
				write = "os.write(1, b'x' * (700 << 10)); os.write(2, b'y' * (700 << 10))"
			}
			executable := probeFixture(t, root, "#!/usr/bin/python3\nimport os\n"+write+"\n")
			var output []byte
			var diagnostic string
			var err error
			if strings.HasPrefix(lane, "version-") {
				output, diagnostic, err = codexVersion(context.Background(), executable, probeFixtureDigest(t, executable), filepath.Join(root, "home"))
				if len(output) > 8<<10 || len(diagnostic) > 64<<10 {
					t.Fatal("version diagnostics exceeded their bounds")
				}
			} else {
				output, err = codexCommand(context.Background(), executable, probeFixtureDigest(t, executable), filepath.Join(root, "home"), "features", "list")
				if len(output) > 1<<20 {
					t.Fatal("combined diagnostics exceeded their total bound")
				}
			}
			if !errors.Is(err, ErrProbeOutputLimit) || errors.Is(err, ErrProbeStopUnconfirmed) {
				t.Fatalf("overflow did not fail with confirmed cleanup: %v", err)
			}
		})
	}
}

func TestCodexProbeDeadlineStopsInheritedPipeDescendants(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, mode := range []string{"parent-deadline", "own-budget"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			heartbeat := filepath.Join(root, "heartbeat")
			executable := probeFixture(t, root, `#!/usr/bin/python3
import os, sys, time
if os.fork() == 0:
 os.setsid()
 with open(sys.argv[1], 'ab', buffering=0) as out:
  until=time.monotonic()+3
  while time.monotonic()<until:
   out.write(b'x');time.sleep(.005)
 os._exit(0)
time.sleep(3)
`)
			ctx := context.Background()
			budget := probeBudget{timeout: 400 * time.Millisecond, stdoutBytes: 1024}
			if mode == "parent-deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 400*time.Millisecond)
				defer cancel()
				budget.timeout = 5 * time.Second
			}
			start := time.Now()
			_, _, err := runCodexProbe(ctx, executable, probeFixtureDigest(t, executable), filepath.Join(root, "home"), budget, heartbeat)
			if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrProbeStopUnconfirmed) || time.Since(start) > 2*time.Second {
				t.Fatalf("deadline did not stop/join the scope within budget: %v (%v)", err, time.Since(start))
			}
			assertProbeHeartbeatStopped(t, heartbeat)
		})
	}
}

func TestCodexProbeFastExitDrainsOutputAndStopsDescendants(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	root := t.TempDir()
	heartbeat := filepath.Join(root, "heartbeat")
	executable := probeFixture(t, root, `#!/usr/bin/python3
import os, sys, time
if os.fork() == 0:
 os.setsid()
 with open(sys.argv[1], 'ab', buffering=0) as out:
  until=time.monotonic()+3
  while time.monotonic()<until:
   out.write(b'x');time.sleep(.005)
 os._exit(0)
until=time.monotonic()+2
while (not os.path.exists(sys.argv[1]) or os.path.getsize(sys.argv[1])==0) and time.monotonic()<until: time.sleep(.002)
print('codex-cli 0.157.1', flush=True)
`)
	start := time.Now()
	output, _, err := runCodexProbe(context.Background(), executable, probeFixtureDigest(t, executable), filepath.Join(root, "home"), probeBudget{
		timeout: 2 * time.Second, stdoutBytes: 1024,
	}, heartbeat)
	if err != nil || string(output) != "codex-cli 0.157.1\n" || time.Since(start) > time.Second {
		t.Fatalf("fast parent exit lost output or waited for leaked descendants: %q %v (%v)", output, err, time.Since(start))
	}
	assertProbeHeartbeatStopped(t, heartbeat)
}

func assertProbeHeartbeatStopped(t *testing.T, path string) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil || len(before) == 0 {
		t.Fatalf("fixture descendant never wrote heartbeat: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("descendant wrote after probe returned", err)
	}
}

func TestPinnedEntrypointsRejectBeforeVersionExecution(t *testing.T) {
	for _, lane := range []string{"live-saved", "live-wif", "engineering-saved", "engineering-wif"} {
		for _, drift := range []string{"digest", "relative", "symlink", "writable", "cancelled"} {
			t.Run(lane+"/"+drift, func(t *testing.T) {
				root := t.TempDir()
				work, home, credentials := filepath.Join(root, "work"), filepath.Join(root, "home"), filepath.Join(root, "credentials")
				for _, dir := range []string{work, home, credentials} {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				login, token := filepath.Join(credentials, "auth.json"), filepath.Join(credentials, "token")
				if err := os.WriteFile(login, []byte(`{"synthetic_fixture":"not_a_real_credential"}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(token, unsignedTimingAssertion(t, time.Now(), time.Now().Add(5*time.Minute)), 0600); err != nil {
					t.Fatal(err)
				}
				// The marker is relative to the executable, avoiding command interpolation.
				script := "#!/usr/bin/python3\nimport pathlib\npathlib.Path(__file__).with_name('executed').write_text('unexpected')\nprint('codex-cli 0.157.1')\n"
				executable := probeFixture(t, root, script)
				digest := canonical.BytesDigest([]byte(script))
				switch drift {
				case "digest":
					digest = "sha256:" + strings.Repeat("a", 64)
				case "relative":
					cwd, err := os.Getwd()
					if err != nil {
						t.Fatal(err)
					}
					executable, err = filepath.Rel(cwd, executable)
					if err != nil {
						t.Fatal(err)
					}
				case "symlink":
					alias := filepath.Join(root, "alias")
					if err := os.Symlink(executable, alias); err != nil {
						t.Fatal(err)
					}
					executable = alias
				case "writable":
					if err := os.Chmod(executable, 0722); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if drift == "cancelled" {
					cancel()
				}
				qualification := "sha256:" + strings.Repeat("b", 64)
				var err error
				switch lane {
				case "live-saved":
					_, err = LiveSavedLoginProbe(ctx, executable, digest, work, home, login, "fixture-model")
				case "live-wif":
					_, err = LiveWIFProbe(ctx, executable, digest, work, home, "fixture-rule", token, "", "fixture-model")
				case "engineering-saved":
					_, err = EngineeringSavedLoginTurn(ctx, executable, testCodexVersion, digest, qualification, work, home, login, "fixture-model", "fixture prompt", nil)
				case "engineering-wif":
					_, err = EngineeringWIFTurn(ctx, executable, testCodexVersion, digest, qualification, work, home, "fixture-rule", token, "", "fixture-model", "fixture prompt", nil)
				}
				want := map[string]string{
					"digest": "qualified executable bytes changed", "relative": "absolute existing path required",
					"symlink": "symlink path alias", "writable": "trusted executable required",
					"cancelled": "context canceled",
				}[drift]
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("expected binary rejection %q before any scope/credential use: %v", want, err)
				}
				if drift == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal("entrypoint lost the probe cancellation error chain", err)
				}
				if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
					t.Fatal("version probe executed before rejecting binary", err)
				}
				entries, err := os.ReadDir(home)
				if err != nil || len(entries) != 0 {
					t.Fatal("failed pre-launch validation mutated HOME", err)
				}
			})
		}
	}
}

func TestCodexProbeRechecksDigestAfterProviderConstruction(t *testing.T) {
	for _, mode := range []string{"version", "command"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			executable := probeFixture(t, root, "#!/bin/sh\nprintf 'codex-cli 0.157.1\\n'\n")
			provider, err := NewPinnedProvider(executable, probeFixtureDigest(t, executable))
			if err != nil {
				t.Fatal(err)
			}
			probeFixture(t, root, "#!/usr/bin/python3\nimport pathlib\npathlib.Path(__file__).with_name('executed').touch()\nprint('codex-cli 0.157.1')\n")
			if mode == "version" {
				_, _, err = codexVersion(context.Background(), provider.executable, provider.digest, filepath.Join(root, "home"))
			} else {
				_, err = codexCommand(context.Background(), provider.executable, provider.digest, filepath.Join(root, "home"), "features", "list")
			}
			if err == nil || !strings.Contains(err.Error(), "qualified executable bytes changed") {
				t.Fatal("probe accepted drift after provider construction", err)
			}
			if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
				t.Fatal("drifted executable ran", err)
			}
		})
	}
}
