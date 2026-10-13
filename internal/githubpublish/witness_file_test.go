//go:build linux

package githubpublish

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPublisherWitnessIntegrityRejectsAliasedAndSpecialFiles(t *testing.T) {
	for _, kind := range []string{"hardlink", "symlink", "setuid", "setgid", "sticky", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			path := filepath.Join(dir, "witness.json")
			if err := os.WriteFile(path, []byte(`{"ok":true}`), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "hardlink":
				err = os.Link(path, filepath.Join(dir, "alias"))
			case "symlink":
				err = os.Rename(path, filepath.Join(dir, "real"))
				if err == nil {
					err = os.Symlink("real", path)
				}
			case "setuid":
				err = os.Chmod(path, 0600|os.ModeSetuid)
			case "setgid":
				err = os.Chmod(path, 0600|os.ModeSetgid)
			case "sticky":
				err = os.Chmod(path, 0600|os.ModeSticky)
			case "fifo":
				err = os.Remove(path)
				if err == nil {
					err = syscall.Mkfifo(path, 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				OK bool `json:"ok"`
			}
			if readWitness(root, "witness.json", &decoded) == nil {
				t.Fatal("aliased or special witness accepted", kind)
			}
		})
	}
}
func TestPublisherWitnessIntegrityRejectsUnsafeRootModes(t *testing.T) {
	for _, mode := range []os.FileMode{0700 | os.ModeSetuid, 0700 | os.ModeSetgid, 0700 | os.ModeSticky, 0770} {
		dir := t.TempDir()
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := newDispatchJournal(dir); err == nil {
			t.Fatalf("unsafe root mode accepted: %v", mode)
		}
	}
}
func TestPublisherWitnessIntegrityDescriptorOpenDoesNotFollowOrBlock(t *testing.T) {
	dir := t.TempDir()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = syscall.Mkfifo(filepath.Join(dir, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openWitnessRead(r, "fifo")
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	f.Close()
	if err != nil || witnessPrivateFile(info) {
		t.Fatal("special descriptor accepted")
	}
	if err = os.WriteFile(filepath.Join(dir, "real"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if f, err = openWitnessRead(r, "link"); err == nil {
		f.Close()
		t.Fatal("followed final symlink")
	}
}
