package cievidence

import (
	"strings"
	"testing"
)

func fixture() Receipt {
	return Receipt{
		SchemaVersion: 1,
		Repository: "jiying2007/engineering-platform",
		Workflow: "CI",
		Event: "pull_request",
		SourceSHA: strings.Repeat("a", 40),
		TestedSHA: strings.Repeat("b", 40),
		BaseSHA: strings.Repeat("c", 40),
		RunID: 123,
		RunAttempt: 1,
		Jobs: []Job{
			{Name:"codex-app-server-0.155.0-qualification",ID:1,Conclusion:"success"},
			{Name:"go",ID:2,Conclusion:"success"},
			{Name:"offline-container-integration",ID:3,Conclusion:"success"},
		},
		Artifacts: []Artifact{
			{Name:"codex-0.155.0-qualification-x",ID:4,Digest:"sha256:"+strings.Repeat("d",64),Size:10},
			{Name:"engineering-binaries-x",ID:5,Digest:"sha256:"+strings.Repeat("e",64),Size:20},
		},
		Files: []File{
			{Path:"codex-qualifier",Digest:"sha256:"+strings.Repeat("1",64),Size:1},
			{Path:"control-plane",Digest:"sha256:"+strings.Repeat("2",64),Size:1},
			{Path:"eng",Digest:"sha256:"+strings.Repeat("3",64),Size:1},
			{Path:"sandbox-guard",Digest:"sha256:"+strings.Repeat("4",64),Size:1},
			{Path:"worker",Digest:"sha256:"+strings.Repeat("5",64),Size:1},
		},
	}
}
func TestEnvelopeRoundTrip(t *testing.T) {
	r:=fixture()
	e,err:=NewEnvelope(r)
	if err!=nil { t.Fatal(err) }
	if err:=e.Verify(); err!=nil { t.Fatal(err) }
	e.Receipt.Files[0].Size++
	if err:=e.Verify(); err==nil { t.Fatal("tamper accepted") }
}
func TestReceiptRejectsMissingOrNonSuccessFacts(t *testing.T) {
	for _,kind:=range []string{"job","artifact","file","base"} {
		t.Run(kind,func(t *testing.T){
			r:=fixture()
			switch kind {
			case "job": r.Jobs[1].Conclusion="failure"
			case "artifact": r.Artifacts=r.Artifacts[:1]
			case "file": r.Files=r.Files[:4]
			case "base": r.BaseSHA=""
			}
			if _,err:=r.Digest(); err==nil { t.Fatal("invalid evidence accepted") }
		})
	}
}
func TestSortMakesOrderCanonical(t *testing.T) {
	r:=fixture()
	r.Jobs[0],r.Jobs[2]=r.Jobs[2],r.Jobs[0]
	r.Artifacts[0],r.Artifacts[1]=r.Artifacts[1],r.Artifacts[0]
	r.Files[0],r.Files[4]=r.Files[4],r.Files[0]
	Sort(&r)
	if err:=r.Validate(); err!=nil { t.Fatal(err) }
}
