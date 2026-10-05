//go:build linux

package artifactset

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// A host-compiler reference case, NOT MCU/hardware qualification or a new
// Runtime execution grant. Generated ELF/map live outside the source tree.
func TestOutOfTreeBuildOutputsSurviveIndependentRestore(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Fatal("C compiler required for build-artifact regression")
	}
	root := t.TempDir()
	must(t, os.Chmod(root, 0700))
	source, build, store := filepath.Join(root, "source"), filepath.Join(root, "build"), filepath.Join(root, "store")
	for _, p := range []string{source, build, store} {
		must(t, os.Mkdir(p, 0700))
	}
	c := []byte("int main(void) { return 0; }\n")
	must(t, os.WriteFile(filepath.Join(source, "main.c"), c, 0400))
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, cc, filepath.Join(source, "main.c"), "-o", filepath.Join(build, "firmware.elf"), "-Wl,-Map,"+filepath.Join(build, "firmware.map"))
	cmd.Dir = build
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reference compile: %v %s", err, out)
	}
	files, err := os.ReadDir(source)
	must(t, err)
	actual, err := os.ReadFile(filepath.Join(source, "main.c"))
	must(t, err)
	if len(files) != 1 || string(actual) != string(c) {
		t.Fatal("build changed source identity")
	}
	p := Plan{Version: 1, Subject: Subject{RunID: "out-of-tree-test", ExecutionID: strings.Repeat("a", 64), TaskDigest: canonical.BytesDigest(c), InputDigest: canonical.BytesDigest(c), BaseCommit: strings.Repeat("b", 40)}}
	for _, name := range []string{"firmware.elf", "firmware.map"} {
		path := filepath.Join(build, name)
		raw, err := os.ReadFile(path)
		must(t, err)
		p.Members = append(p.Members, Input{Entry: Entry{ID: name, Kind: "build-output", Size: int64(len(raw)), Digest: canonical.BytesDigest(raw)}, Path: path})
	}
	plan, sum := writePlan(t, build, p)
	archive := filepath.Join(store, "build.tar")
	r, err := Pack(ctx, plan, sum, archive)
	must(t, err)
	must(t, os.RemoveAll(source))
	must(t, os.RemoveAll(build))
	target := filepath.Join(store, "restore")
	_, err = Restore(ctx, archive, r.ArchiveDigest, p.Subject.RunID, target)
	must(t, err)
	for _, e := range p.Members {
		raw, err := os.ReadFile(filepath.Join(target, "files", e.ID))
		must(t, err)
		if canonical.BytesDigest(raw) != e.Digest {
			t.Fatal("output changed")
		}
		st, err := os.Stat(filepath.Join(target, "files", e.ID))
		must(t, err)
		if st.Mode().Perm() != 0600 {
			t.Fatal("executable automatically enabled")
		}
	}
}
