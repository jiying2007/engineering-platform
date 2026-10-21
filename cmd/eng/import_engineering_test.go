package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/engineeringevidence"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func terminalGitFixture(t *testing.T, extraChange bool) (string, string, string) {
	t.Helper()
	plan, err := production.BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--object-format=sha1")
	git("config", "user.name", "terminal Git test")
	git("config", "user.email", "test@example.invalid")
	marker := filepath.Join(repo, plan.Plan.MarkerPath)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{marker: plan.Plan.MarkerBefore, filepath.Join(repo, "unrelated.txt"): "unchanged\n"} {
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-m", "base")
	base := git("rev-parse", "HEAD")
	if err := os.WriteFile(marker, []byte(plan.Plan.MarkerAfter), 0o644); err != nil {
		t.Fatal(err)
	}
	if extraChange {
		if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-m", "result")
	return repo, base, git("rev-parse", "HEAD")
}

func TestGitChangeManifestTerminalFlagEnforcesFrozenChange(t *testing.T) {
	for _, tc := range []struct {
		name            string
		terminal, extra bool
		wantError       bool
	}{
		{name: "terminal-exact", terminal: true},
		{name: "terminal-extra-rejected", terminal: true, extra: true, wantError: true},
		{name: "ordinary-extra-preserved", extra: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, base, result := terminalGitFixture(t, tc.extra)
			output := filepath.Join(t.TempDir(), "manifest.json")
			args := []string{"--repository", repo, "--base", base, "--result", result, "--output", output}
			if tc.terminal {
				args = append(args, "--terminal")
			}
			err := gitChangeManifest(args)
			if tc.wantError {
				if err == nil {
					t.Fatal("terminal CLI accepted another changed file")
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("failed terminal capture retained a manifest")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			retained, _, err := engineeringevidence.ReadGitChangeManifest(output)
			if err != nil || retained.SchemaVersion != 1 || retained.BaseCommit != base || retained.ResultCommit != result {
				t.Fatalf("wrong retained v1 manifest: %#v %v", retained, err)
			}
		})
	}
}

func TestImportGitChangeEvidenceAutomaticallyConstrainsTerminalTarget(t *testing.T) {
	plan, err := production.BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                   string
		extra, ordinary, drift bool
		wantPosts              int32
	}{
		{name: "terminal-exact", wantPosts: 1},
		{name: "terminal-extra-rejected", extra: true},
		{name: "ordinary-extra-preserved", ordinary: true, extra: true, wantPosts: 1},
		{name: "target-subject-drift-rejected", extra: true, drift: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, base, result := terminalGitFixture(t, tc.extra)
			// A caller can retain an ordinary manifest without --terminal; the
			// authenticated importer must still enforce the terminal Delivery.
			manifest, err := engineeringevidence.CaptureGitChange(context.Background(), repo, base, result)
			if err != nil {
				t.Fatal(err)
			}
			data, err := engineeringevidence.MarshalGitChangeManifest(manifest)
			if err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			delivery := core.DeliveryReceipt{
				ID: "delivery-terminal", TaskContractDigest: "sha256:" + strings.Repeat("a", 64),
				RunID: "run-terminal", TargetID: plan.Plan.TargetID, BaseCommit: base, ResultCommit: result,
				Artifacts: []core.ArtifactRef{{ID: "git-change", Digest: canonical.BytesDigest(data), MediaType: engineeringevidence.GitArtifactType}},
			}
			if tc.ordinary {
				delivery.TargetID = "ordinary-target"
			}
			delivery.SubjectDigest, err = delivery.CalculateSubjectDigest()
			if err != nil {
				t.Fatal(err)
			}
			if tc.drift {
				delivery.TargetID = "ordinary-target"
			}
			pki := testsupport.NewPKI(t)
			var gets, posts atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/deliveries/"+delivery.ID:
					gets.Add(1)
					_ = json.NewEncoder(w).Encode(delivery)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/evidence":
					posts.Add(1)
					var request struct {
						DeliveryReceiptID string           `json:"delivery_receipt_id"`
						Evidence          core.EvidenceRef `json:"evidence"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.DeliveryReceiptID != delivery.ID ||
						request.Evidence.SubjectDigest != delivery.SubjectDigest || request.Evidence.Procedure != engineeringevidence.GitProcedure {
						t.Error("importer changed the existing Evidence contract")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(request.Evidence)
				default:
					t.Error("unexpected importer request")
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			cert := pki.ClientCertificate(t, engineeringevidence.GitImporter)
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for name, raw := range map[string][]byte{
				"ca.pem":     pki.CAPEM,
				"client.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}),
				"client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}),
			} {
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("CONTROL_ENDPOINT", server.URL)
			t.Setenv("CONTROL_CLIENT_CERT_FILE", filepath.Join(dir, "client.pem"))
			t.Setenv("CONTROL_CLIENT_KEY_FILE", filepath.Join(dir, "client.key"))
			t.Setenv("CONTROL_SERVER_CA_FILE", filepath.Join(dir, "ca.pem"))
			err = importGitChangeEvidence([]string{"--repository", repo, "--manifest", manifestPath, "--delivery", delivery.ID, "--requirement", "req-git", "--evidence", "evidence-git", "--artifact", "git-change"})
			if (err == nil) != (tc.wantPosts == 1) || gets.Load() != 1 || posts.Load() != tc.wantPosts {
				t.Fatalf("import result err=%v GET=%d POST=%d want POST=%d", err, gets.Load(), posts.Load(), tc.wantPosts)
			}
		})
	}
}
