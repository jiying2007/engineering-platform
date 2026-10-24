package access

import (
	"reflect"
	"testing"
)

func TestCapacityWorkerGrantsUseCurrentClaimAuthorityAndDetachedCopies(t *testing.T) {
	p, err := New(Document{Version: 1, Principals: []PrincipalSpec{
		{Subject: "urn:engineering-platform:worker:z", Scope: "platform", Capabilities: []string{WorkerPoll, WorkerReport, WorkerPrepare}, WorkerProfiles: []string{"worker/z", "worker/y"}},
		{Subject: "urn:engineering-platform:worker:a", Scope: "platform", Capabilities: []string{WorkerPoll}, WorkerProfiles: []string{"worker/a"}},
		{Subject: "urn:engineering-platform:worker:report-only", Scope: "platform", Capabilities: []string{WorkerReport}, WorkerProfiles: []string{"worker/retired"}},
		{Subject: "urn:engineering-platform:operator", Scope: "platform", Capabilities: []string{Read}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	expected := []WorkerClaimGrant{
		{Subject: "urn:engineering-platform:worker:a", Profiles: []string{"worker/a"}},
		{Subject: "urn:engineering-platform:worker:z", Profiles: []string{"worker/y", "worker/z"}, Prepares: true},
	}
	got := p.WorkerClaimGrants()
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("claim authority inventory: %#v", got)
	}
	got[0].Profiles[0] = "worker/injected"
	got[1].Subject = "urn:engineering-platform:forged"
	if !reflect.DeepEqual(p.WorkerClaimGrants(), expected) {
		t.Fatal("read view mutated original policy")
	}
}
