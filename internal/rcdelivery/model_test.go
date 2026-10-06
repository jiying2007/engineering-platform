package rcdelivery

import (
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/cievidence"
	"github.com/jiying2007/engineering-platform/internal/distribution"
)

func fixture(t *testing.T) cievidence.Envelope {
	t.Helper()
	sha := strings.Repeat("a", 40)
	r := cievidence.Receipt{
		SchemaVersion: cievidence.SchemaVersion, Repository: cievidence.TrustedRepository,
		Workflow: "CI", Event: "push", SourceSHA: sha, TestedSHA: sha,
		RunID: 123, RunAttempt: 1,
		Jobs: []cievidence.Job{
			{Name: "codex-app-server-qualification", ID: 1, Conclusion: "success"},
			{Name: "go", ID: 2, Conclusion: "success"},
			{Name: "offline-container-integration", ID: 3, Conclusion: "success"},
			{Name: "postgres-authority-restore-drill", ID: 4, Conclusion: "success"},
		},
		Artifacts: []cievidence.Artifact{
			{Name: "codex-compatibility-qualification-" + sha, ID: 10, Digest: canonical.BytesDigest([]byte("codex")), Size: 5},
			{Name: "engineering-binaries-" + sha, ID: 11, Digest: canonical.BytesDigest([]byte("binaries")), Size: 8},
		},
	}
	for _, name := range distribution.Names() {
		r.Files = append(r.Files, cievidence.File{Path: name, Digest: canonical.BytesDigest([]byte(name)), Size: int64(len(name))})
	}
	cievidence.Sort(&r)
	e, err := cievidence.NewEnvelope(r)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestBuildBindsInternalRCWithoutPromotingExternalGates(t *testing.T) {
	ci := fixture(t)
	e, err := Build(ci, strings.Repeat("b", 40), []byte("# Implementation Status\ncurrent\n"), []byte("{}\n"), []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Verify(); err != nil {
		t.Fatal(err)
	}
	if e.Delivery.ProviderLiveExecuted || e.Delivery.HumanReviewPerformed || e.Delivery.ProductionQualified ||
		e.Delivery.CIEvidenceDigest != ci.ReceiptDigest ||
		e.Delivery.BinaryArtifact.Name != "engineering-binaries-"+ci.Receipt.SourceSHA ||
		len(e.Delivery.TerminalRequiredGates) == 0 {
		t.Fatal("RC envelope overclaimed or lost exact identities", e)
	}
}

func TestBuildRejectsPRDriftAndArbitraryGovernanceBytes(t *testing.T) {
	tests := []func(*cievidence.Envelope, *string, *[]byte){
		func(ci *cievidence.Envelope, _ *string, _ *[]byte) {
			ci.Receipt.Event = "pull_request"
			ci.Receipt.BaseSHA = strings.Repeat("c", 40)
			ci.ReceiptDigest, _ = ci.Receipt.Digest()
		},
		func(ci *cievidence.Envelope, _ *string, _ *[]byte) {
			ci.Receipt.TestedSHA = strings.Repeat("d", 40)
			ci.ReceiptDigest, _ = ci.Receipt.Digest()
		},
		func(_ *cievidence.Envelope, tree *string, _ *[]byte) { *tree = "not-a-tree" },
		func(_ *cievidence.Envelope, _ *string, status *[]byte) { *status = []byte("other\n") },
	}
	for i, mutate := range tests {
		ci := fixture(t)
		tree := strings.Repeat("b", 40)
		status := []byte("# Implementation Status\ncurrent\n")
		mutate(&ci, &tree, &status)
		if _, err := Build(ci, tree, status, []byte("{}\n"), []byte("{}\n")); err == nil {
			t.Fatal("invalid RC input accepted", i)
		}
	}
}

func TestEnvelopeMutationIsRejected(t *testing.T) {
	ci := fixture(t)
	e, err := Build(ci, strings.Repeat("b", 40), []byte("# Implementation Status\ncurrent\n"), []byte("{}\n"), []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	e.Delivery.ProductionQualified = true
	if e.Verify() == nil {
		t.Fatal("production claim accepted")
	}
}
