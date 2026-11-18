package local

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if MaybeRunChild() {
		return
	}
	os.Exit(m.Run())
}

func executionRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "fixture"}, {"config", "user.email", "fixture@example.invalid"}} {
		if _, err := gitOutput(context.Background(), root, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := gitOutput(context.Background(), root, "add", "source.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitOutput(context.Background(), root, "commit", "-m", "baseline"); err != nil {
		t.Fatal(err)
	}
	return root
}

func executionTask(t *testing.T, store *Store, root, script string, checks []Check, artifacts []string, model bool, timeout string) Task {
	t.Helper()
	config, digest, err := Freeze(Project{Name: "fixture", Root: root, Base: "main", Runtime: "fixture", Checks: checks, Artifacts: artifacts}, Runtime{Name: "fixture", Argv: []string{"/bin/sh", "-c", script}, Stdin: "task", Model: model, Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.CreateTask(context.Background(), Task{Project: "fixture", Owner: "test", Instruction: "after\n", Config: config, ConfigDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func executionStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func executionOwner(t *testing.T, store *Store) *Lock {
	t.Helper()
	lock, err := AcquireScheduler(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	return lock
}

func lastExecutionAttempt(t *testing.T, store *Store, task Task) Attempt {
	t.Helper()
	attempts, err := store.ListAttempts(context.Background(), task.ID)
	if err != nil || len(attempts) == 0 {
		t.Fatalf("attempts: %v, %v", attempts, err)
	}
	return attempts[len(attempts)-1]
}

func TestExecuteRealGitStdinChecksAndArtifacts(t *testing.T) {
	store, root := executionStore(t), executionRepository(t)
	executionOwner(t, store)
	check := Check{Name: "source", Argv: []string{"/bin/sh", "-c", "grep -qx after source.txt && mkdir -p out && printf verified > out/result.txt"}, Timeout: "5s"}
	task := executionTask(t, store, root, "cat > source.txt; printf runtime-output; printf diagnostic >&2", []Check{check}, []string{"out/**/*.txt"}, false, "5s")
	finished, err := Execute(context.Background(), store, task.ID)
	if err != nil || finished.State != StateDone {
		t.Fatalf("execute: %+v, %v", finished, err)
	}
	attempt := lastExecutionAttempt(t, store, finished)
	r := attempt.Result
	if !r.RuntimeStarted || !r.LocalProcessStopped || r.RemoteOutcomeUnknown || r.RuntimeExitCode != 0 || r.ChecksStatus != "passed" || len(r.Checks) != 1 || len(r.Artifacts) != 1 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.Checks[0].SourceCommit != r.ResultCommit || r.StartCommit != finished.BaseSHA || r.ResultCommit == r.StartCommit || attempt.ConfigDigest != task.ConfigDigest {
		t.Fatalf("commit/config provenance lost: %+v", r)
	}
	original, _ := os.ReadFile(filepath.Join(root, "source.txt"))
	if string(original) != "before\n" {
		t.Fatalf("original checkout changed: %q", original)
	}
	diff, err := os.ReadFile(r.DiffPath)
	if err != nil || !strings.Contains(string(diff), "+after") || strings.Contains(string(diff), "out/result.txt") {
		t.Fatalf("candidate diff does not match verified source: %q, %v", diff, err)
	}
	artifact, err := os.ReadFile(r.Artifacts[0].Path)
	if err != nil || string(artifact) != "verified" || !strings.HasPrefix(r.Artifacts[0].Digest, "sha256:") {
		t.Fatalf("artifact: %q, %v", artifact, err)
	}
	stdout, _ := os.ReadFile(r.RuntimeStdout)
	stderr, _ := os.ReadFile(r.RuntimeStderr)
	if string(stdout) != "runtime-output" || string(stderr) != "diagnostic" {
		t.Fatalf("logs: stdout=%q stderr=%q", stdout, stderr)
	}
	if r.Usage.Known || attempt.RuntimeVersion != "unknown (not probed)" {
		t.Fatal("fixture must not invent cost or provider version")
	}
}

func TestExecuteNoChecksRequiresAcceptanceAndArtifacts(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual-review", true: "missing-artifact"}[missing], func(t *testing.T) {
			store, root := executionStore(t), executionRepository(t)
			executionOwner(t, store)
			var artifacts []string
			if missing {
				artifacts = []string{"missing.bin"}
			}
			task := executionTask(t, store, root, "cat > source.txt", nil, artifacts, false, "5s")
			finished, err := Execute(context.Background(), store, task.ID)
			if err != nil || finished.State != StateWaiting {
				t.Fatalf("execute: %+v, %v", finished, err)
			}
			result := lastExecutionAttempt(t, store, task).Result
			if result.ManualAcceptanceReady == missing || result.ChecksStatus != "not_configured" {
				t.Fatalf("manual acceptance readiness: %+v", result)
			}
			accepted, acceptErr := store.AcceptTask(context.Background(), task.ID, "Read the produced source", "tester")
			if missing && acceptErr == nil || !missing && (acceptErr != nil || accepted.State != StateDone) {
				t.Fatalf("accept: %+v, %v", accepted, acceptErr)
			}
		})
	}
}

func TestExecuteCheckFailureAndSourceMutation(t *testing.T) {
	for _, fixture := range []struct {
		name, script, state string
		exit                int
	}{
		{"nonzero", "printf failure >&2; exit 17", StateFailed, 17},
		{"source-change", "printf altered > source.txt", StateWaiting, 0},
		{"new-source", "printf altered > new-source.txt", StateWaiting, 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store, root := executionStore(t), executionRepository(t)
			executionOwner(t, store)
			task := executionTask(t, store, root, "cat > source.txt", []Check{{Name: "verify", Argv: []string{"/bin/sh", "-c", fixture.script}, Timeout: "5s"}}, nil, false, "5s")
			finished, err := Execute(context.Background(), store, task.ID)
			if err != nil || finished.State != fixture.state {
				t.Fatalf("execute: %+v, %v", finished, err)
			}
			r := lastExecutionAttempt(t, store, task).Result
			if len(r.Checks) != 1 || r.Checks[0].ExitCode != fixture.exit || r.ManualAcceptanceReady {
				t.Fatalf("wrong real check result: %+v", r)
			}
			if fixture.state == StateWaiting && r.ChecksStatus == "passed" {
				t.Fatal("modified bytes cannot inherit the previous candidate's checks")
			}
		})
	}
}

func TestExecuteTimeoutUnknownAndExplicitResume(t *testing.T) {
	store, root := executionStore(t), executionRepository(t)
	executionOwner(t, store)
	// This is deliberately a local sleep fixture, labelled as a model adapter to
	// exercise uncertainty policy. It makes no provider request.
	task := executionTask(t, store, root, "cat >/dev/null; printf invocation >> calls.txt; sleep 20", nil, nil, true, "150ms")
	finished, err := Execute(context.Background(), store, task.ID)
	if err != nil || finished.State != StateWaiting {
		t.Fatalf("timeout: %+v, %v", finished, err)
	}
	first := lastExecutionAttempt(t, store, task)
	if !first.Result.RemoteOutcomeUnknown || !first.Result.LocalProcessStopped || first.Result.Usage.Known {
		t.Fatalf("timeout fabricated certainty: %+v", first.Result)
	}
	if _, err := Recover(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	attempts, _ := store.ListAttempts(context.Background(), task.ID)
	if len(attempts) != 1 {
		t.Fatal("recover replayed the runtime")
	}
	if _, err := Execute(context.Background(), store, task.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("implicit replay accepted: %v", err)
	}
	if _, err := store.EnqueueResume(context.Background(), task.ID, "second explicit attempt"); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(context.Background(), store, task.ID); err != nil {
		t.Fatal(err)
	}
	attempts, _ = store.ListAttempts(context.Background(), task.ID)
	if len(attempts) != 2 || attempts[0].Input != "after\n" || attempts[1].Input != "second explicit attempt" || attempts[1].StartCommit != attempts[0].Result.ResultCommit {
		t.Fatalf("resume overwrote or restarted old input: %+v", attempts)
	}
}

func TestExecuteStopReleasesLocalSlotWithoutInventingRemoteOutcome(t *testing.T) {
	store, root := executionStore(t), executionRepository(t)
	executionOwner(t, store)
	task := executionTask(t, store, root, "cat >/dev/null; printf ready > start.flag; sleep 30", nil, nil, true, "40s")
	type completion struct {
		task Task
		err  error
	}
	done := make(chan completion, 1)
	go func() {
		finished, err := Execute(context.Background(), store, task.ID)
		done <- completion{finished, err}
	}()
	deadline := time.Now().Add(5 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(store.DataDir(), "tasks", task.ID, "worktree", "start.flag")); err == nil {
			started = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !started {
		store.RequestStop(context.Background(), task.ID)
		t.Fatal("fixture did not start")
	}
	if err := StopTask(context.Background(), store, task.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case finished := <-done:
		if finished.err != nil || finished.task.State != StateCancelled {
			t.Fatalf("stop: %+v, %v", finished.task, finished.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not reach the process group")
	}
	prior := lastExecutionAttempt(t, store, task)
	if !prior.Result.RemoteOutcomeUnknown || !prior.Result.LocalProcessStopped {
		t.Fatalf("local stop changed remote uncertainty: %+v", prior.Result)
	}
	next := executionTask(t, store, root, "cat > source.txt", []Check{{Name: "verify", Argv: []string{"/bin/sh", "-c", "grep -qx after source.txt"}, Timeout: "5s"}}, nil, false, "5s")
	finished, err := Execute(context.Background(), store, next.ID)
	if err != nil || finished.State != StateDone {
		t.Fatalf("remote uncertainty incorrectly blocked other work: %+v, %v", finished, err)
	}
}

func TestSupervisionGateFailureNeverRunsCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "must-not-exist")
	out := runSupervised(context.Background(), []string{"/bin/sh", "-c", "touch must-not-exist"}, dir, "", os.Environ(), filepath.Join(dir, "out"), filepath.Join(dir, "err"), nil, func(ProcessIdentity) error { return errors.New("fixture persistence failure") })
	if out.Err == nil || out.Started || !out.Stopped {
		t.Fatalf("gate result: %+v", out)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unrecorded command executed: %v", err)
	}
}

func TestSupervisionStopsDescendantsAfterLeaderExit(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := runSupervised(ctx, []string{"/bin/sh", "-c", "sleep 20 & exit 0"}, dir, "", os.Environ(), filepath.Join(dir, "out"), filepath.Join(dir, "err"), nil, func(ProcessIdentity) error { return nil })
	if out.Err == nil || !out.Stopped || !out.Started || out.Identity == nil {
		t.Fatalf("unobserved descendants: %+v", out)
	}
	if alive, err := ProcessGroupAlive(*out.Identity); err != nil || alive {
		t.Fatalf("descendants left running: %v, %v", alive, err)
	}
}

func TestProcessIdentityMismatchNeverSignalsReplacement(t *testing.T) {
	child := exec.Command("/bin/sleep", "30")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	identity, err := identifyProcess(child.Process.Pid)
	if err != nil {
		child.Process.Kill()
		child.Wait()
		t.Fatal(err)
	}
	defer func() { stopProcessGroup(identity, 100*time.Millisecond); child.Wait() }()
	previous := identity
	previous.StartTicks++ // Saved identity belongs to a different process lifetime.
	if alive, err := ProcessGroupAlive(previous); err != nil || alive {
		t.Fatalf("reused PID kept an ended identity unresolved: %t, %v", alive, err)
	}
	if stopped, err := stopProcessGroup(previous, 100*time.Millisecond); err != nil || !stopped {
		t.Fatalf("previous identity did not settle as stopped: %t, %v", stopped, err)
	}
	if alive, err := ProcessGroupAlive(identity); err != nil || !alive {
		t.Fatalf("stop signalled the replacement process: %t, %v", alive, err)
	}
}

func TestRepositoryAndSchedulerLocksCannotBeBypassed(t *testing.T) {
	store, root := executionStore(t), executionRepository(t)
	owner := executionOwner(t, store)
	if second, err := AcquireScheduler(store.DataDir()); err == nil {
		second.Close()
		t.Fatal("two schedulers acquired one data directory")
	}
	lock, info, err := AcquireRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	second, aliased, err := AcquireRepository(context.Background(), alias)
	if err == nil {
		second.Close()
		t.Fatal("repository alias bypassed common-dir lock")
	}
	if aliased.CommonDir != info.CommonDir {
		t.Fatal("alias was not canonicalized")
	}
	if !owner.operation.TryLock() {
		t.Fatal("unused scheduler operation unexpectedly held")
	}
	if _, err := Recover(context.Background(), store); !errors.Is(err, ErrConflict) {
		t.Fatalf("same process concurrent scheduler operation allowed: %v", err)
	}
	owner.operation.Unlock()
}

// Invoked in a separate copy of this test binary by the crash recovery test.
func TestExecutionCrashHelper(t *testing.T) {
	dataDir := os.Getenv("ENG_EXECUTION_CRASH_DATA")
	if dataDir == "" {
		return
	}
	store, err := OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := AcquireScheduler(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := Execute(context.Background(), store, os.Getenv("ENG_EXECUTION_CRASH_TASK")); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionSupervisorCrashDoesNotReplay(t *testing.T) {
	for _, closeFDs := range []bool{false, true} {
		t.Run(map[bool]string{false: "inherited-locks", true: "runtime-closes-locks"}[closeFDs], func(t *testing.T) {
			store, root := executionStore(t), executionRepository(t)
			script := "cat >/dev/null; printf ready > start.flag; sleep 30"
			if closeFDs {
				script = "exec 4>&-; exec 5>&-; " + script
			}
			task := executionTask(t, store, root, script, nil, nil, true, "40s")
			child := exec.Command(os.Args[0], "-test.run=^TestExecutionCrashHelper$")
			child.Env = append(os.Environ(), "ENG_EXECUTION_CRASH_DATA="+store.DataDir(), "ENG_EXECUTION_CRASH_TASK="+task.ID)
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			var process *ProcessIdentity
			started := false
			t.Cleanup(func() {
				child.Process.Kill()
				if process != nil {
					stopProcessGroup(*process, 100*time.Millisecond)
				}
			})
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				attempts, err := store.ListAttempts(context.Background(), task.ID)
				if err == nil && len(attempts) > 0 && attempts[0].Process != nil {
					process = attempts[0].Process
					if _, err := os.Stat(filepath.Join(store.DataDir(), "tasks", task.ID, "worktree", "start.flag")); err == nil {
						started = true
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if process == nil || !started {
				t.Fatal("fixture runtime did not start")
			}
			if err := child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = child.Wait()
			if closeFDs {
				owner := executionOwner(t, store)
				if _, err := Recover(context.Background(), store); err != nil {
					t.Fatal(err)
				}
				prior := lastExecutionAttempt(t, store, task)
				if prior.Result.LocalProcessStopped || !prior.Result.RemoteOutcomeUnknown {
					t.Fatalf("recovery forgot original running group: %+v", prior.Result)
				}
				if next, _, err := AcquireRepository(context.Background(), root); !errors.Is(err, ErrProcessUnresolved) {
					if next != nil {
						next.Close()
					}
					t.Fatalf("cross-data-dir common-dir record bypassed: %v", err)
				}
				owner.Close()
			} else if owner, err := AcquireScheduler(store.DataDir()); err == nil {
				owner.Close()
				t.Fatal("inherited lock released while original runtime remained active")
			}
			if closeFDs {
				// The process can finish outside eng after the first recovery. The
				// next recovery must refresh waiting, without requiring a new run.
				if stopped, err := stopProcessGroup(*process, 100*time.Millisecond); err != nil || !stopped {
					t.Fatalf("fixture termination: %t, %v", stopped, err)
				}
			} else if err := StopTask(context.Background(), store, task.ID); err != nil {
				t.Fatal(err)
			}
			executionOwner(t, store)
			if _, err := Recover(context.Background(), store); err != nil {
				t.Fatal(err)
			}
			finished, _ := store.GetTask(context.Background(), task.ID)
			attempts, _ := store.ListAttempts(context.Background(), task.ID)
			if finished.State != StateWaiting || len(attempts) != 1 || !attempts[0].Result.LocalProcessStopped || !attempts[0].Result.RemoteOutcomeUnknown {
				t.Fatalf("crash recovery invented replay/success: %+v, %+v", finished, attempts)
			}
		})
	}
}
