package access

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedPilotAccessPolicyLeastPrivilege(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "pilots", "access-policy.json.tmpl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const (
		profileDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		workerProfile = "worker/codex-pilot"
	)
	rendered := strings.ReplaceAll(string(data), "__PROFILE_DIGEST__", profileDigest)
	rendered = strings.ReplaceAll(rendered, "__WORKER_PROFILE__", workerProfile)
	if strings.Contains(rendered, "__") {
		t.Fatal("unrendered pilot access-policy placeholder")
	}
	policy, err := Decode([]byte(rendered))
	if err != nil {
		t.Fatal(err)
	}

	worker := policy.principals["urn:engineering-platform:worker:codex-pilot"]
	if !worker.Allows(WorkerPoll) || !worker.Allows(WorkerReport) || !worker.Allows(WorkerPrepare) ||
		!worker.Allows(ActionExecute) || !worker.AllowsWorkerProfile(workerProfile) ||
		!worker.AllowsAction("worker.codex-execute", "CONTROLLED_MUTATION", profileDigest) {
		t.Fatalf("Codex Worker lacks exact retained-pilot grant: %#v", worker)
	}
	if worker.Allows(Read) || worker.Allows(ActionReconcile) ||
		worker.AllowsAction("github.publish-pr", "CONTROLLED_MUTATION", "github.publish-pr") {
		t.Fatal("Codex Worker gained publication/read/reconciliation authority")
	}

	publisher := policy.principals["urn:engineering-platform:operator:pilot-publisher"]
	if !publisher.Allows(Read) || !publisher.Allows(ActionExecute) || !publisher.Allows(ActionReconcile) ||
		!publisher.AllowsAction("github.publish-pr", "CONTROLLED_MUTATION", "github.publish-pr") {
		t.Fatalf("publisher requester lacks exact publication grant: %#v", publisher)
	}
	if publisher.AllowsAction("worker.codex-execute", "CONTROLLED_MUTATION", profileDigest) ||
		len(publisher.workerProfiles) != 0 {
		t.Fatal("publisher requester gained Worker/Codex authority")
	}

	for _, item := range []struct {
		subject   string
		issuer    string
		procedure string
	}{
		{TrustedCIImporterSubject, TrustedCIIssuer, TrustedCIProcedure},
		{GitEvidenceImporterSubject, GitEvidenceIssuer, GitEvidenceProcedure},
		{CodexEvidenceImporterSubject, CodexEvidenceIssuer, CodexEvidenceProcedure},
	} {
		id := policy.principals[item.subject]
		if !id.Allows(Read) || !id.Allows(EvidenceRegister) ||
			!id.AllowsEvidence(item.issuer, item.procedure) ||
			len(id.capabilities) != 2 || len(id.actions) != 0 || len(id.workerProfiles) != 0 {
			t.Fatalf("reserved importer authority drift for %s: %#v", item.subject, id)
		}
	}

	reviewer := policy.principals["urn:engineering-platform:reviewer:pilot"]
	if !reviewer.Allows(Read) || !reviewer.Allows(ReviewCreate) || len(reviewer.capabilities) != 2 {
		t.Fatalf("reviewer is not isolated: %#v", reviewer)
	}
	verifier := policy.principals["urn:engineering-platform:verifier:pilot"]
	if !verifier.Allows(Read) || !verifier.Allows(VerificationCreate) || len(verifier.capabilities) != 2 {
		t.Fatalf("verifier is not isolated: %#v", verifier)
	}
	closure := policy.principals["urn:engineering-platform:closure:pilot"]
	if !closure.Allows(Read) || !closure.Allows(ClosureCreate) || len(closure.capabilities) != 2 {
		t.Fatalf("closure authority is not isolated: %#v", closure)
	}
}
