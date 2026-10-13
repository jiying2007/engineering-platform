package production

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/githubpublish"
)

func publisherHostFixture(t *testing.T) (githubpublish.PlanConfiguration, githubpublish.ServiceConfiguration, map[uint32]bool) {
	t.Helper()
	root := t.TempDir()
	artifact, witness := filepath.Join(root, "artifacts"), filepath.Join(root, "witness")
	for _, p := range []string{artifact, witness} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	targets := []githubpublish.TargetPolicy{{Repository: "fixture/project", BaseRef: "main", BranchPrefix: "engineering/"}}
	plan := githubpublish.PlanConfiguration{Version: 1, ArtifactRoot: artifact, Targets: targets}
	pub := githubpublish.ServiceConfiguration{Version: 2, ArtifactRoot: artifact, WitnessRoot: witness, Targets: targets,
		GitExecutable: writeExecutable(t, root, "git"), TokenFile: writeEnv(t, root, "token", "fixture-token\n")}
	groups := map[uint32]bool{uint32(os.Getgid()): true}
	for _, g := range mustGroups(t) {
		groups[uint32(g)] = true
	}
	return plan, pub, groups
}

func TestPublisherHostSchemaMatchesServiceV2(t *testing.T) {
	plan, pub, groups := publisherHostFixture(t)
	raw, err := json.Marshal(pub)
	if err != nil {
		t.Fatal(err)
	}
	path := writeEnv(t, t.TempDir(), "publisher.json", string(raw))
	// Reproduce the former mismatch using the exact former reader type: a valid
	// v2 configuration cannot be decoded as the legacy in-process configuration.
	var legacy githubpublish.Configuration
	if readHostJSON(path, &legacy) == nil {
		t.Fatal("legacy reader unexpectedly accepted witness_root")
	}
	var observed githubpublish.ServiceConfiguration
	if err := readHostJSON(path, &observed); err != nil {
		t.Fatal(err)
	}
	if err := checkPublisherPolicyAndWitness(plan, observed, os.Getuid(), groups); err != nil {
		t.Fatal(err)
	}
	if _, err := githubpublish.LoadRemoteService(path, "urn:engineering-platform:control:publisher-client"); err != nil {
		t.Fatal(err)
	}
}
func TestPublisherHostWitnessOwnershipAndSeparation(t *testing.T) {
	for _, kind := range []string{"legacy", "wrong-owner", "group-access", "setgid", "alias", "same-root", "inside-artifact", "contains-artifact", "target-drift"} {
		t.Run(kind, func(t *testing.T) {
			plan, pub, groups := publisherHostFixture(t)
			uid := os.Getuid()
			switch kind {
			case "legacy":
				pub.Version = 1
			case "wrong-owner":
				uid += 10000
			case "group-access":
				if err := os.Chmod(pub.WitnessRoot, 0770); err != nil {
					t.Fatal(err)
				}
			case "setgid":
				if err := os.Chmod(pub.WitnessRoot, 0700|os.ModeSetgid); err != nil {
					t.Fatal(err)
				}
			case "alias":
				alias := pub.WitnessRoot + "-link"
				if err := os.Symlink(pub.WitnessRoot, alias); err != nil {
					t.Fatal(err)
				}
				pub.WitnessRoot = alias
			case "same-root":
				pub.WitnessRoot = pub.ArtifactRoot
			case "inside-artifact":
				pub.WitnessRoot = filepath.Join(pub.ArtifactRoot, "private")
				if err := os.Mkdir(pub.WitnessRoot, 0700); err != nil {
					t.Fatal(err)
				}
			case "contains-artifact":
				pub.WitnessRoot = filepath.Dir(pub.ArtifactRoot)
			case "target-drift":
				pub.Targets = []githubpublish.TargetPolicy{{Repository: "fixture/project", BaseRef: "other", BranchPrefix: "engineering/"}}
			}
			if err := checkPublisherPolicyAndWitness(plan, pub, uid, groups); err == nil {
				t.Fatal("unsafe Publisher host accepted")
			}
		})
	}
}
