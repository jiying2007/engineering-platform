package cievidence

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

type importFixture struct {
	repository string
	runID      int64
	sourceSHA  string
	testedSHA  string
	baseSHA    string
	envelope   Envelope
	zip        []byte
	artifacts  []githubArtifact
	delivery   core.DeliveryReceipt
	jobs       githubJobs
}

func newImportFixture(t *testing.T) importFixture {
	t.Helper()
	f := importFixture{
		repository: "owner/repo",
		runID:      42,
		sourceSHA:  strings.Repeat("a", 40),
		testedSHA:  strings.Repeat("b", 40),
		baseSHA:    strings.Repeat("c", 40),
	}
	receipt := Receipt{
		SchemaVersion: SchemaVersion,
		Repository:    f.repository,
		Workflow:      "CI",
		Event:         "pull_request",
		SourceSHA:     f.sourceSHA,
		TestedSHA:     f.testedSHA,
		BaseSHA:       f.baseSHA,
		RunID:         f.runID,
		RunAttempt:    1,
		Jobs: []Job{
			{Name: "codex-app-server-0.155.0-qualification", ID: 201, Conclusion: "success"},
			{Name: "go", ID: 202, Conclusion: "success"},
			{Name: "offline-container-integration", ID: 203, Conclusion: "success"},
		},
		Artifacts: []Artifact{
			{Name: "codex-0.155.0-qualification-" + f.testedSHA, ID: 101, Digest: "sha256:" + strings.Repeat("1", 64), Size: 782},
			{Name: "engineering-binaries-" + f.testedSHA, ID: 102, Digest: "sha256:" + strings.Repeat("2", 64), Size: 22 << 20},
		},
		Files: []File{
			{Path: "codex-qualifier", Digest: "sha256:" + strings.Repeat("3", 64), Size: 1},
			{Path: "control-plane", Digest: "sha256:" + strings.Repeat("4", 64), Size: 1},
			{Path: "eng", Digest: "sha256:" + strings.Repeat("5", 64), Size: 1},
			{Path: "sandbox-guard", Digest: "sha256:" + strings.Repeat("6", 64), Size: 1},
			{Path: "worker", Digest: "sha256:" + strings.Repeat("7", 64), Size: 1},
		},
	}
	var err error
	f.envelope, err = NewEnvelope(receipt)
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, err := json.Marshal(f.envelope)
	if err != nil {
		t.Fatal(err)
	}
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	w, err := zw.Create("ci-evidence-envelope.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(envelopeJSON); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.zip = zipped.Bytes()

	for _, fact := range receipt.Artifacts {
		f.artifacts = append(f.artifacts, githubArtifact{
			ID:      fact.ID,
			Name:    fact.Name,
			Size:    fact.Size,
			Digest:  fact.Digest,
			Expired: false,
			Workflow: struct {
				ID      int64  `json:"id"`
				HeadSHA string `json:"head_sha"`
			}{ID: f.runID, HeadSHA: f.testedSHA},
		})
	}
	trusted := githubArtifact{
		ID:      103,
		Name:    "trusted-ci-evidence-" + f.testedSHA,
		Size:    int64(len(f.zip)),
		Digest:  canonical.BytesDigest(f.zip),
		Expired: false,
		Workflow: struct {
			ID      int64  `json:"id"`
			HeadSHA string `json:"head_sha"`
		}{ID: f.runID, HeadSHA: f.testedSHA},
	}
	f.artifacts = append(f.artifacts, trusted)
	for _, job := range receipt.Jobs {
		f.jobs.Jobs = append(f.jobs.Jobs, struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}{ID: job.ID, Name: job.Name, Status: "completed", Conclusion: "success"})
	}
	f.delivery = core.DeliveryReceipt{
		ID:            "delivery-1",
		BaseCommit:    f.baseSHA,
		ResultCommit:  f.sourceSHA,
		SubjectDigest: "sha256:" + strings.Repeat("8", 64),
	}
	for _, artifact := range f.artifacts {
		f.delivery.Artifacts = append(f.delivery.Artifacts, core.ArtifactRef{
			ID:        GitHubArtifactID(f.runID, artifact.ID),
			Digest:    artifact.Digest,
			MediaType: GitHubArtifactMediaType,
		})
	}
	return f
}

func (f importFixture) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing read token on GitHub API request")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/" + f.repository + "/actions/runs/42":
			var run githubRun
			run.ID = f.runID
			run.Name = "CI"
			run.Event = "pull_request"
			run.Status = "completed"
			run.Conclusion = "success"
			run.HeadSHA = f.testedSHA
			run.RunAttempt = 1
			run.Pulls = make([]struct {
				Head struct {
					SHA string `json:"sha"`
				} `json:"head"`
				Base struct {
					SHA string `json:"sha"`
				} `json:"base"`
			}, 1)
			run.Pulls[0].Head.SHA = f.sourceSHA
			run.Pulls[0].Base.SHA = f.baseSHA
			_ = json.NewEncoder(w).Encode(run)
		case "/repos/" + f.repository + "/actions/runs/42/jobs":
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("jobs query not bounded")
			}
			_ = json.NewEncoder(w).Encode(f.jobs)
		case "/repos/" + f.repository + "/actions/runs/42/artifacts":
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("artifacts query not bounded")
			}
			_ = json.NewEncoder(w).Encode(githubArtifacts{Artifacts: f.artifacts})
		case "/repos/" + f.repository + "/actions/artifacts/103/zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(f.zip)
		default:
			http.NotFound(w, r)
		}
	})
}

func verifierForFixture(t *testing.T, f importFixture) *githubVerifier {
	t.Helper()
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &githubVerifier{base: base, client: server.Client(), token: "test-token"}
}

func TestGitHubImportBindsLivePRFactsToExactDelivery(t *testing.T) {
	f := newImportFixture(t)
	verifier := verifierForFixture(t, f)
	evidence, envelope, err := verifier.VerifyPullRequest(context.Background(), f.repository, f.runID, f.delivery, "req-ci")
	if err != nil {
		t.Fatal(err)
	}
	if evidence.DeliveryReceiptID != f.delivery.ID || evidence.RequirementID != "req-ci" || evidence.SubjectDigest != f.delivery.SubjectDigest || evidence.Issuer != GitHubIssuer || evidence.Procedure != GitHubProcedure || evidence.Result != "PASS" || !evidence.Applicable {
		t.Fatalf("unexpected evidence: %#v", evidence)
	}
	if len(evidence.ArtifactRefs) != 3 || envelope.ReceiptDigest != f.envelope.ReceiptDigest || !strings.HasPrefix(evidence.ID, "github-ci-") {
		t.Fatalf("missing retained provenance: %#v %#v", evidence, envelope)
	}
}

func TestGitHubImportRejectsDeliveryAndLiveFactSubstitution(t *testing.T) {
	for _, kind := range []string{"result-commit", "base-commit", "artifact", "job", "zip-digest", "expired"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t)
			switch kind {
			case "result-commit":
				f.delivery.ResultCommit = strings.Repeat("d", 40)
			case "base-commit":
				f.delivery.BaseCommit = strings.Repeat("d", 40)
			case "artifact":
				f.delivery.Artifacts[0].Digest = "sha256:" + strings.Repeat("f", 64)
			case "job":
				f.jobs.Jobs[1].Conclusion = "failure"
			case "zip-digest":
				f.artifacts[2].Digest = "sha256:" + strings.Repeat("e", 64)
			case "expired":
				f.artifacts[2].Expired = true
			}
			verifier := verifierForFixture(t, f)
			if _, _, err := verifier.VerifyPullRequest(context.Background(), f.repository, f.runID, f.delivery, "req-ci"); err == nil {
				t.Fatal("substituted CI evidence accepted")
			}
		})
	}
}

func TestReadEnvelopeZIPRejectsExtraOrWrongEntry(t *testing.T) {
	for _, names := range [][]string{{"wrong.json"}, {"ci-evidence-envelope.json", "extra"}} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, name := range names {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte("{}"))
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := readEnvelopeZIP(buf.Bytes()); err == nil {
			t.Fatal("unsafe evidence ZIP accepted")
		}
	}
}

func TestGitHubArtifactIDIsStable(t *testing.T) {
	if got := GitHubArtifactID(42, 103); got != "github-actions/run/42/artifact/103" {
		t.Fatal(got)
	}
}
