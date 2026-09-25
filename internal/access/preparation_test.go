package access

import "testing"

func TestPreparationRequiresDedicatedGrantAndExistingWorkerCapabilities(t *testing.T) {
	for _, caps := range [][]string{{WorkerPrepare}, {WorkerPrepare, WorkerPoll}, {WorkerPrepare, WorkerReport}} {
		if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{{Subject: "urn:engineering-platform:worker:prep", Scope: "platform", Capabilities: caps, WorkerProfiles: []string{"worker/prepare"}}}}); err == nil {
			t.Fatal("incomplete preparation authority accepted")
		}
	}
	if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{{Subject: "urn:engineering-platform:worker:prep", Scope: "platform", Capabilities: []string{WorkerPrepare, WorkerPoll, WorkerReport}, WorkerProfiles: []string{"worker/prepare"}}}}); err != nil {
		t.Fatal(err)
	}
}
