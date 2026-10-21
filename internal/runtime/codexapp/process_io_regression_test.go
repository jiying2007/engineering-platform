package codexapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// These subprocesses use the local TestMain protocol fixture, never a provider.
func TestRuntimeRetainsFinalCompletionOnImmediateExit(t *testing.T) {
	for _, lane := range []string{"live-saved", "live-wif", "engineering-saved", "engineering-wif"} {
		t.Run(lane, func(t *testing.T) {
			engineering := strings.HasPrefix(lane, "engineering-")
			if engineering {
				testsupport.RequireProcessNamespaces(t)
			}
			root := t.TempDir()
			work, home, credentials := filepath.Join(root, "work"), filepath.Join(root, "home"), filepath.Join(root, "credentials")
			for _, dir := range []string{work, home, credentials} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(work, ".fixture-exit-after-completion"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			login := filepath.Join(credentials, "auth.json")
			if err := os.WriteFile(login, []byte(`{"synthetic_fixture":"not_a_real_credential"}`), 0600); err != nil {
				t.Fatal(err)
			}
			token := filepath.Join(credentials, "identity-token")
			now := time.Now()
			if err := os.WriteFile(token, unsignedTimingAssertion(t, now, now.Add(5*time.Minute)), 0600); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			executable, err = filepath.EvalSymlinks(executable)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := executableDigest(executable)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if engineering {
				var receipt EngineeringReceipt
				qualification := canonical.BytesDigest([]byte("synthetic-fixture-qualification"))
				if strings.HasSuffix(lane, "-wif") {
					receipt, err = EngineeringWIFTurn(ctx, executable, testCodexVersion, digest, qualification, work, home, "fixture-rule", token, "", "gpt-test", "Fixture-only engineering turn.", nil)
				} else {
					receipt, err = EngineeringSavedLoginTurn(ctx, executable, testCodexVersion, digest, qualification, work, home, login, "gpt-test", "Fixture-only engineering turn.", nil)
				}
				if err != nil || receipt.Validate() != nil || !receipt.ProcessScope.Quiescent() || receipt.Output != "fixture engineering change complete" {
					t.Fatalf("final engineering result lost: %+v %v", receipt, err)
				}
			} else {
				var receipt LiveReceipt
				if strings.HasSuffix(lane, "-wif") {
					receipt, err = LiveWIFProbe(ctx, executable, digest, work, home, "fixture-rule", token, "", "gpt-test")
				} else {
					receipt, err = LiveSavedLoginProbe(ctx, executable, digest, work, home, login, "gpt-test")
				}
				if err != nil || receipt.Validate() != nil || receipt.Output != LiveProbeExpected {
					t.Fatalf("final live result lost: %+v %v", receipt, err)
				}
			}
		})
	}
}

func TestLiveProbeStartupErrorHasConcurrentBoundedDiagnostics(t *testing.T) {
	for _, lane := range []string{"saved", "wif"} {
		t.Run(lane, func(t *testing.T) {
			root := t.TempDir()
			work, home, credentials := filepath.Join(root, "work"), filepath.Join(root, "home"), filepath.Join(root, "credentials")
			for _, dir := range []string{work, home, credentials} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			login := filepath.Join(credentials, "auth.json")
			if err := os.WriteFile(login, []byte(`{"synthetic_fixture":"not_a_real_credential"}`), 0600); err != nil {
				t.Fatal(err)
			}
			token := filepath.Join(credentials, "identity-token")
			if err := os.WriteFile(token, []byte("fixture-only-not-a-real-identity-token"), 0600); err != nil {
				t.Fatal(err)
			}
			script := []byte(`#!/usr/bin/python3
import sys,json,threading,time
if sys.argv[1:] == ["--version"]:
 print("codex-cli 0.157.1");sys.exit(0)
def diagnostic_writer():
 while True:
  sys.stderr.write("fixture diagnostic\n");sys.stderr.flush();time.sleep(0.0001)
threading.Thread(target=diagnostic_writer,daemon=True).start()
request=json.loads(sys.stdin.readline())
time.sleep(0.01)
print(json.dumps({"id":request["id"],"error":{"code":-32000,"message":"synthetic startup failure"}}),flush=True)
time.sleep(10)
`)
			executable := filepath.Join(root, "synthetic-codex")
			if err := os.WriteFile(executable, script, 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var err error
			if lane == "wif" {
				_, err = LiveWIFProbe(ctx, executable, canonical.BytesDigest(script), work, home, "fixture-rule", token, "", "synthetic-only-model")
			} else {
				_, err = LiveSavedLoginProbe(ctx, executable, canonical.BytesDigest(script), work, home, login, "synthetic-only-model")
			}
			var rpc *RPCError
			if !errors.As(err, &rpc) || rpc.Code != -32000 || !strings.Contains(err.Error(), "fixture diagnostic") || len(err.Error()) > (65<<10) {
				t.Fatalf("expected bounded concurrent startup diagnostic: %v", err)
			}
		})
	}
}
