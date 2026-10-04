package distribution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalRoles(t *testing.T) {
	names := Names()
	last := ""
	publisher := false
	for _, name := range names {
		if name <= last || strings.ContainsAny(name, "/\\.") {
			t.Fatal("invalid role contract")
		}
		last = name
		publisher = publisher || name == "publisher-service"
	}
	if !publisher {
		t.Fatal("publisher omitted")
	}
	names[0] = "mutated"
	if Names()[0] == "mutated" {
		t.Fatal("mutable contract leaked")
	}
}
func TestRejectMissingExtraAndAliasedDistribution(t *testing.T) {
	dir := t.TempDir()
	if _, err := Verify(dir); err == nil {
		t.Fatal("empty package accepted")
	}
	alias := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(alias); err == nil {
		t.Fatal("symlink package accepted")
	}
}
