package access

import (
	"encoding/json"
	"strings"
	"testing"

	productionassets "github.com/jiying2007/engineering-platform/examples/production"
)

func routingPrincipal(subject, profile string, prepare bool) PrincipalSpec {
	caps := []string{WorkerPoll, WorkerReport}
	if prepare {
		caps = append(caps, WorkerPrepare)
	}
	return PrincipalSpec{Subject: "urn:engineering-platform:worker:" + subject, Scope: "platform", Capabilities: caps, WorkerProfiles: []string{profile}}
}

func TestWorkerRoutingRejectsMixedClaimersBeforePolicyPublication(t *testing.T) {
	admission := routingPrincipal("admission", "worker/shared", false)
	preparation := routingPrincipal("preparation", "worker/shared", true)
	for _, specs := range [][]PrincipalSpec{{admission, preparation}, {preparation, admission}} {
		if p, err := New(Document{Version: 1, Principals: specs}); err == nil || p != nil {
			t.Fatal("mixed admission/preparation claimers published a policy")
		}
		raw, err := json.Marshal(Document{Version: 1, Principals: specs})
		if err != nil {
			t.Fatal(err)
		}
		if p, err := Decode(raw); err == nil || p != nil {
			t.Fatal("decoded policy bypassed routing separation")
		}
	}
	// A second grant can collide even when each actor has a different first grant.
	admission.WorkerProfiles = []string{"worker/admission", "worker/shared"}
	preparation.WorkerProfiles = []string{"worker/preparation", "worker/shared"}
	if p, err := New(Document{Version: 1, Principals: []PrincipalSpec{admission, preparation}}); err == nil || p != nil {
		t.Fatal("secondary profile collision ignored")
	}
}

func TestWorkerRoutingKeepsSameLaneReplicasAndExactGrants(t *testing.T) {
	for _, prepare := range []bool{false, true} {
		a, b := routingPrincipal("first", "worker/shared", prepare), routingPrincipal("second", "worker/shared", prepare)
		if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{a, b}}); err != nil {
			t.Fatal("same-lane replicas rejected", err)
		}
	}
	a, b := routingPrincipal("admission", "worker/admission", false), routingPrincipal("preparation", "worker/preparation", true)
	// A report-only actor cannot claim work, so it does not create this race.
	reporter := routingPrincipal("report-only", "worker/preparation", false)
	reporter.Capabilities = []string{WorkerReport}
	p, err := New(Document{Version: 1, Principals: []PrincipalSpec{a, b, reporter}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []PrincipalSpec{a, b} {
		id := p.principals[s.Subject]
		if !id.AllowsWorkerProfile(s.WorkerProfiles[0]) || id.AllowsWorkerProfile("worker/unlisted") || id.Allows(ActionExecute) {
			t.Fatal("grant broadened")
		}
	}
	a.WorkerProfiles[0] = "worker/preparation"
	if p.principals[a.Subject].AllowsWorkerProfile("worker/preparation") {
		t.Fatal("caller mutation changed published grants")
	}
}

func TestCanonicalProductionWorkerProfilesAreDisjoint(t *testing.T) {
	files, err := productionassets.Files()
	if err != nil {
		t.Fatal(err)
	}
	profiles := map[string]string{}
	for role, mode := range map[string]string{"admission": "--admission-only", "preparation": "--prepare-only"} {
		text := string(files["systemd/engineering-worker-"+role+".service"])
		for _, line := range strings.Split(text, "\n") {
			if !strings.HasPrefix(line, "ExecStart=") {
				continue
			}
			parts := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			if len(parts) != 3 || parts[0] != "/opt/engineering-platform/bin/worker" || parts[2] != mode || !strings.HasPrefix(parts[1], "--profile=") {
				t.Fatal("canonical Worker invocation drift", role)
			}
			profiles[role] = strings.TrimPrefix(parts[1], "--profile=")
		}
	}
	if profiles["admission"] != "worker/admission-production" || profiles["preparation"] != "worker/codex-production" {
		t.Fatal("canonical claim lanes overlap or drift", profiles)
	}
	if _, err := New(Document{Version: 1, Principals: []PrincipalSpec{routingPrincipal("admission", profiles["admission"], false), routingPrincipal("preparation", profiles["preparation"], true)}}); err != nil {
		t.Fatal(err)
	}
}
