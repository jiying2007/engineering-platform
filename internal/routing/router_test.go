package routing

import "testing"

func TestLinuxDebugRoute(t *testing.T) {
	r, err := Resolve("DEBUG", "Linux UBI storage")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.SkillIDs) != 3 || r.SkillIDs[2] != "linux-bsp-debug" {
		t.Fatalf("unexpected route: %#v", r)
	}
}

func TestUnsupportedTaskFailsClosed(t *testing.T) {
	if _, err := Resolve("MAGIC", "anything"); err == nil {
		t.Fatal("expected unsupported task error")
	}
}
