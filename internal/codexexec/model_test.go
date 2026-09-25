package codexexec

import (
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

func contractFixture(t *testing.T) (Profile, Permit) {
	t.Helper()
	p := Profile{Version: 1, CodexVersion: codexapp.QualifiedCodexVersion, BinaryDigest: "sha256:"+strings.Repeat("a",64), EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), Model: "gpt-test", Sandbox: "workspace-write", ApprovalPolicy: "never"}
	pd, err := p.Digest(); if err != nil { t.Fatal(err) }
	task := core.TaskContract{ID:"task",WorkItemID:"work",TaskType:"FEATURE",Repository:"repo",BaseCommit:strings.Repeat("1",40),AcceptanceCriteria:[]string{"change code","tests pass"},AllowedActions:[]string{Action},ExpectedOutputs:[]string{"source change"},VerificationPlanID:"vp",VerificationPlanDigest:"sha256:"+strings.Repeat("2",64),Revision:1}
	td, err := task.Digest(); if err != nil { t.Fatal(err) }
	input := core.RunInputManifest{RunID:"run",TaskContractDigest:td,RuntimeProfile:"codex/runtime",ToolProfile:"codex/"+pd,WorkerProfile:"worker/codex",PolicyProfile:"policy"}
	id, err := input.Digest(); if err != nil { t.Fatal(err) }
	intent := workerqueue.Intent{RunID:"run",TaskDigest:td,InputDigest:id,ExecutionEpoch:1}
	intentDigest, err := intent.Digest(); if err != nil { t.Fatal(err) }
	wt := workerqueue.Token{Profile:"worker/codex",InboxID:1,Generation:1,RecoveryEpoch:0}
	a := workerqueue.Assignment{Token:wt,LeaseUntil:time.Unix(100,0),Intent:intent,IntentDigest:intentDigest,Input:input,Task:task}
	validation, err := workerqueue.Validate(a); if err != nil { t.Fatal(err) }
	manifest := contextbundle.Manifest{SchemaVersion:1,RunInputDigest:id}
	rawDigest := canonical.BytesDigest([]byte(`{"schema_version":1,"run_input_manifest_digest":"`+id+`","entries":null}`))
	// Marshal rather than rely on hand formatting if the manifest representation changes.
	if got, err := canonical.Digest(manifest); err == nil && got == "" { t.Fatal("unreachable", got) }
	_ = rawDigest
	manifestRaw, err := canonicalJSON(manifest); if err != nil { t.Fatal(err) }
	bundleDigest := canonical.BytesDigest(manifestRaw)
	facts := preparation.Facts{Version:1,IntentDigest:intentDigest,InputDigest:id,TaskDigest:td,ApprovalDigest:"sha256:"+strings.Repeat("3",64),BaseCommit:task.BaseCommit,TreeCommit:strings.Repeat("4",40),WorkspaceRecipe:workspace.Recipe,SourceDigest:"sha256:"+strings.Repeat("5",64),ConfigDigest:"sha256:"+strings.Repeat("6",64),BundleDigest:bundleDigest,Context:manifest}
	fd, err := canonical.Digest(facts); if err != nil { t.Fatal(err) }
	worker := "urn:engineering-platform:worker:codex"
	prep := preparation.Receipt{Kind:preparation.Kind,Admission:workerqueue.Receipt{Token:wt,Worker:worker,Kind:workerqueue.Validated,Validation:validation,ReceivedAt:time.Unix(10,0)},Facts:facts,FactsDigest:fd,ReceivedAt:time.Unix(11,0)}
	return p, Permit{Token:Token{ID:strings.Repeat("d",64),RunID:"run",WorkerProfile:"worker/codex",ProfileDigest:pd},Assignment:a,Preparation:prep,Profile:p,LeaseUntil:time.Unix(200,0)}
}

func canonicalJSON(v any) ([]byte,error) {
	return json.Marshal(v)
}

func TestProfileAndAssignmentAreExact(t *testing.T) {
	p, permit := contractFixture(t)
	if err := CheckAssignment(permit.Assignment,p); err != nil { t.Fatal(err) }
	bad := permit.Assignment; bad.Input.ToolProfile = "codex/other"
	if err := CheckAssignment(bad,p); err == nil { t.Fatal("wrong tool profile accepted") }
	p.ToolNetwork = true
	if err := p.Validate(); err == nil { t.Fatal("network-enabled profile accepted") }
}

func TestPromptIdentityIsLocatorIndependent(t *testing.T) {
	_, permit := contractFixture(t)
	one, d1, err := Prompt(permit.Assignment,permit.Preparation,"/one/bundle"); if err != nil { t.Fatal(err) }
	two, d2, err := Prompt(permit.Assignment,permit.Preparation,"/two/bundle"); if err != nil { t.Fatal(err) }
	if d1 != d2 || one == two { t.Fatalf("prompt identity incorrectly includes locator: %s %s",d1,d2) }
	expected, err := PromptIdentityDigest(permit.Assignment,permit.Preparation); if err != nil || expected != d1 { t.Fatal(expected,err) }
}

func TestResultValidationBindsPromptModelAndChange(t *testing.T) {
	p, permit := contractFixture(t)
	promptDigest, _ := PromptIdentityDigest(permit.Assignment,permit.Preparation)
	codex := codexapp.EngineeringReceipt{SchemaVersion:1,CLI:"codex-cli",Version:codexapp.QualifiedCodexVersion,BinaryDigest:p.BinaryDigest,EngineeringConfigDigest:p.EngineeringConfigDigest,CredentialMode:"workload_identity",FederationRuleID:"rule",Model:p.Model,PromptDigest:"sha256:"+strings.Repeat("7",64),ThreadID:"thread",TurnID:"turn",TurnStatus:"completed",Output:"done",OutputDigest:canonical.BytesDigest([]byte("done")),CommandCount:1,FileChangeCount:1,AssertionRemovedBeforeTurn:true}
	change := workspace.ChangeFacts{Recipe:workspace.FinalizeRecipe,BaseCommit:permit.Preparation.Facts.BaseCommit,BaseTree:permit.Preparation.Facts.TreeCommit,BaseSourceDigest:permit.Preparation.Facts.SourceDigest,ResultCommit:strings.Repeat("8",40),ResultTree:strings.Repeat("9",40),ResultSourceDigest:"sha256:"+strings.Repeat("b",64),BundleDigest:"sha256:"+strings.Repeat("c",64),BundleSize:100}
	r := Result{PromptIdentityDigest:promptDigest,Codex:codex,Change:change}
	if err := r.Validate(p,permit); err != nil { t.Fatal(err) }
	r.PromptIdentityDigest = "sha256:"+strings.Repeat("f",64)
	if err := r.Validate(p,permit); err == nil { t.Fatal("forged prompt identity accepted") }
}
