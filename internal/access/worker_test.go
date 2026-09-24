package access

import "testing"

func TestWorkerProfilesAreExplicitAndCopied(t *testing.T) {
	spec := PrincipalSpec{Subject: "urn:engineering-platform:worker:test", Scope: "platform", Capabilities: []string{WorkerPoll, WorkerReport}, WorkerProfiles: []string{"worker/ubuntu"}}
	p, err := New(Document{Version: 1, Principals: []PrincipalSpec{spec}})
	if err != nil {
		t.Fatal(err)
	}
	spec.WorkerProfiles[0] = "worker/other"
	id := p.principals[spec.Subject]
	if !id.AllowsWorkerProfile("worker/ubuntu") || id.AllowsWorkerProfile("worker/other") {
		t.Fatal("profile grant not frozen")
	}
	spec.WorkerProfiles = nil
	if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{spec}}); err == nil {
		t.Fatal("unscoped worker accepted")
	}
	spec.Capabilities = []string{Read}
	spec.WorkerProfiles = []string{"worker/ubuntu"}
	if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{spec}}); err == nil {
		t.Fatal("profile without worker capability accepted")
	}
}
