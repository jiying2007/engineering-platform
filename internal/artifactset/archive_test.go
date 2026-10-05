//go:build linux

package artifactset

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T) (Plan, string, string) {
	t.Helper()
	root := t.TempDir()
	must(t, os.Chmod(root, 0700))
	p := Plan{Version: 1, Subject: Subject{RunID: "test-only-run", ExecutionID: strings.Repeat("a", 64), TaskDigest: canonical.BytesDigest([]byte("test task")), InputDigest: canonical.BytesDigest([]byte("test input")), BaseCommit: strings.Repeat("b", 40)}}
	for i, b := range [][]byte{[]byte("TEST-ONLY-PRIVATE\x00\xff"), {}, bytes.Repeat([]byte("12345678"), 65536)} {
		id := []string{"a-record.json", "b-empty", "c-firmware.bin"}[i]
		path := filepath.Join(root, id)
		must(t, os.WriteFile(path, b, 0600))
		p.Members = append(p.Members, Input{Entry: Entry{ID: id, Kind: "build-output", Size: int64(len(b)), Digest: canonical.BytesDigest(b)}, Path: path})
	}
	path, sum := writePlan(t, root, p)
	return p, path, sum
}
func writePlan(t *testing.T, root string, p Plan) (string, string) {
	t.Helper()
	b, e := json.Marshal(p)
	must(t, e)
	path := filepath.Join(root, "plan.json")
	must(t, os.WriteFile(path, b, 0600))
	return path, canonical.BytesDigest(b)
}
func TestPackRestoreIndependentDeterministic(t *testing.T) {
	p, path, sum := fixture(t)
	store := t.TempDir()
	must(t, os.Chmod(store, 0700))
	ctx := context.Background()
	archive := filepath.Join(store, "set.tar")
	r, e := Pack(ctx, path, sum, archive)
	must(t, e)
	if r.Members != 3 || r.Coverage != Coverage || r.ExecutionAuthorized || r.ProducerSemanticsVerified || r.ProductionQualified {
		t.Fatal(r)
	}
	again := filepath.Join(store, "second.tar")
	r2, e := Pack(ctx, path, sum, again)
	must(t, e)
	if r != r2 {
		t.Fatal("nondeterministic archive")
	}
	original := map[string][]byte{}
	for _, in := range p.Members {
		original[in.ID], e = os.ReadFile(in.Path)
		must(t, e)
	}
	must(t, os.RemoveAll(filepath.Dir(path)))
	target := filepath.Join(store, "restored")
	restored, e := Restore(ctx, archive, r.ArchiveDigest, p.Subject.RunID, target)
	must(t, e)
	if restored.Status != "ARTIFACT_SET_RESTORED_BYTES_VERIFIED" || restored.ExecutionAuthorized {
		t.Fatal(restored)
	}
	for name, want := range original {
		got, e := os.ReadFile(filepath.Join(target, "files", name))
		must(t, e)
		if !bytes.Equal(got, want) {
			t.Fatal(name)
		}
	}
	raw, e := os.ReadFile(filepath.Join(target, "manifest.json"))
	must(t, e)
	if bytes.Contains(raw, []byte("source_path")) || bytes.Contains(raw, []byte("TEST-ONLY-PRIVATE")) {
		t.Fatal("paths or content leaked into manifest")
	}
	if _, e = Restore(ctx, archive, r.ArchiveDigest, p.Subject.RunID, target); e == nil {
		t.Fatal("restoration overwrote existing directory")
	}
	if _, e = Verify(ctx, archive, r.ArchiveDigest, "other-run"); e == nil {
		t.Fatal("accepted wrong run")
	}
}
func TestPlansAndInputsFailClosed(t *testing.T) {
	tests := map[string]func(*testing.T, *Plan){
		"duplicate-id":  func(t *testing.T, p *Plan) { p.Members[1].ID = p.Members[0].ID },
		"unordered":     func(t *testing.T, p *Plan) { p.Members[0], p.Members[1] = p.Members[1], p.Members[0] },
		"traversal-id":  func(t *testing.T, p *Plan) { p.Members[0].ID = "../escaped" },
		"unknown-kind":  func(t *testing.T, p *Plan) { p.Members[0].Kind = "auto-detect" },
		"wrong-size":    func(t *testing.T, p *Plan) { p.Members[0].Size++ },
		"wrong-digest":  func(t *testing.T, p *Plan) { p.Members[0].Digest = "sha256:" + strings.Repeat("0", 64) },
		"negative-size": func(t *testing.T, p *Plan) { p.Members[0].Size = -1 },
		"oversized":     func(t *testing.T, p *Plan) { p.Members[0].Size = MaxFile + 1 },
		"missing":       func(t *testing.T, p *Plan) { must(t, os.Remove(p.Members[0].Path)) },
		"directory":     func(t *testing.T, p *Plan) { p.Members[0].Path = filepath.Dir(p.Members[0].Path) },
		"symlink": func(t *testing.T, p *Plan) {
			path := p.Members[0].Path
			must(t, os.Rename(path, path+"-other"))
			must(t, os.Symlink(path+"-other", path))
		},
		"fifo": func(t *testing.T, p *Plan) {
			path := p.Members[0].Path
			must(t, os.Remove(path))
			must(t, syscall.Mkfifo(path, 0600))
		},
		"hardlink":           func(t *testing.T, p *Plan) { must(t, os.Link(p.Members[0].Path, p.Members[0].Path+"-alias")) },
		"writable-by-others": func(t *testing.T, p *Plan) { must(t, os.Chmod(p.Members[0].Path, 0666)) },
		"public-parent":      func(t *testing.T, p *Plan) { must(t, os.Chmod(filepath.Dir(p.Members[0].Path), 0755)) },
		"same-file-twice":    func(t *testing.T, p *Plan) { p.Members[1].Path = p.Members[0].Path },
		"relative-file":      func(t *testing.T, p *Plan) { p.Members[0].Path = "file" },
		"too-many":           func(t *testing.T, p *Plan) { p.Members = make([]Input, MaxMembers+1) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			p, path, _ := fixture(t)
			change(t, &p)
			path, sum := writePlan(t, filepath.Dir(path), p)
			dir := t.TempDir()
			must(t, os.Chmod(dir, 0700))
			out := filepath.Join(dir, "set.tar")
			if _, e := Pack(context.Background(), path, sum, out); e == nil {
				t.Fatal("accepted bad input")
			}
			if _, e := os.Lstat(out); !os.IsNotExist(e) {
				t.Fatal("failed pack published archive")
			}
		})
	}
}
func TestDeclarationAndDestinationConstraints(t *testing.T) {
	_, path, sum := fixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	must(t, os.Chmod(dir, 0700))
	out := filepath.Join(dir, "set.tar")
	if _, e := Pack(ctx, path, "sha256:"+strings.Repeat("0", 64), out); e == nil {
		t.Fatal("wrong external plan digest accepted")
	}
	raw, e := os.ReadFile(path)
	must(t, e)
	for _, bad := range [][]byte{append([]byte(`{"version":1,`), raw[1:]...), append([]byte(`{"execution_authorized":true,`), raw[1:]...), append(raw, []byte("{}")...)} {
		must(t, os.WriteFile(path, bad, 0600))
		if _, e := Pack(ctx, path, canonical.BytesDigest(bad), out); e == nil {
			t.Fatal("ambiguous declaration accepted")
		}
	}
	must(t, os.WriteFile(path, raw, 0600))
	must(t, os.WriteFile(out, []byte("keep existing"), 0600))
	if _, e := Pack(ctx, path, sum, out); e == nil {
		t.Fatal("overwrite")
	}
	got, e := os.ReadFile(out)
	must(t, e)
	if string(got) != "keep existing" {
		t.Fatal("overwritten")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = Pack(canceled, path, sum, out+"new"); e == nil {
		t.Fatal("cancellation ignored")
	}
}
func rewriteArchive(t *testing.T, raw []byte, change func(int, *tar.Header, []byte) (*tar.Header, []byte)) []byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(raw))
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for i := 0; ; i++ {
		h, e := tr.Next()
		if e != nil {
			break
		}
		data, e := io.ReadAll(tr)
		must(t, e)
		h, data = change(i, h, data)
		h.Size = int64(len(data))
		must(t, tw.WriteHeader(h))
		_, e = tw.Write(data)
		must(t, e)
	}
	must(t, tw.Close())
	return b.Bytes()
}
func TestArchiveTamperingRehashedStillRejected(t *testing.T) {
	p, path, sum := fixture(t)
	dir := t.TempDir()
	must(t, os.Chmod(dir, 0700))
	out := filepath.Join(dir, "set.tar")
	r, e := Pack(context.Background(), path, sum, out)
	must(t, e)
	raw, e := os.ReadFile(out)
	must(t, e)
	cases := map[string][]byte{"truncated": raw[:len(raw)-1024], "trailing": append(append([]byte{}, raw...), make([]byte, 512)...), "bad-magic": append([]byte("x"), raw[1:]...)}
	changes := map[string]func(int, *tar.Header, []byte) (*tar.Header, []byte){
		"symlink": func(i int, h *tar.Header, b []byte) (*tar.Header, []byte) {
			if i == 1 {
				h.Typeflag = tar.TypeSymlink
				h.Linkname = "/etc/passwd"
				b = nil
			}
			return h, b
		},
		"path-traversal": func(i int, h *tar.Header, b []byte) (*tar.Header, []byte) {
			if i == 1 {
				h.Name = "../escape"
			}
			return h, b
		},
		"payload": func(i int, h *tar.Header, b []byte) (*tar.Header, []byte) {
			if i == 1 {
				b[0] ^= 1
			}
			return h, b
		},
		"manifest-claim": func(i int, h *tar.Header, b []byte) (*tar.Header, []byte) {
			if i == 0 {
				b = append([]byte(`{"execution_authorized":true,`), b[1:]...)
			}
			return h, b
		},
		"duplicate-member": func(i int, h *tar.Header, b []byte) (*tar.Header, []byte) {
			if i == 2 {
				h.Name = "files/" + p.Members[0].ID
			}
			return h, b
		},
		"executable": func(i int, h *tar.Header, b []byte) (*tar.Header, []byte) {
			if i == 1 {
				h.Mode = 0700
			}
			return h, b
		},
	}
	for n, c := range changes {
		cases[n] = rewriteArchive(t, raw, c)
	}
	for n, b := range cases {
		t.Run(n, func(t *testing.T) {
			bad := filepath.Join(dir, "bad-"+n)
			must(t, os.WriteFile(bad, b, 0600))
			if _, e := Verify(context.Background(), bad, canonical.BytesDigest(b), p.Subject.RunID); e == nil {
				t.Fatal("accepted rehashed malformed archive")
			}
			if _, e := Restore(context.Background(), bad, canonical.BytesDigest(b), p.Subject.RunID, bad+"-restore"); e == nil {
				t.Fatal("restored malformed archive")
			}
		})
	}
	if _, e = Verify(context.Background(), out, "sha256:"+strings.Repeat("0", 64), p.Subject.RunID); e == nil {
		t.Fatal("wrong anchor")
	}
	if _, e = Verify(context.Background(), out, r.ArchiveDigest, p.Subject.RunID); e != nil {
		t.Fatal("original changed", e)
	}
}
func TestConcurrentPublicationNeverOverwrites(t *testing.T) {
	p, path, sum := fixture(t)
	dir := t.TempDir()
	must(t, os.Chmod(dir, 0700))
	out := filepath.Join(dir, "set.tar")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := Pack(context.Background(), path, sum, out); results <- e }()
	}
	wg.Wait()
	close(results)
	pass := 0
	for e := range results {
		if e == nil {
			pass++
		}
	}
	if pass != 1 {
		t.Fatalf("wanted one successful publication, got %d", pass)
	}
	d, _, e := fileDigest(context.Background(), out)
	must(t, e)
	_, e = Verify(context.Background(), out, d, p.Subject.RunID)
	must(t, e)
	files, e := os.ReadDir(dir)
	must(t, e)
	if len(files) != 1 {
		t.Fatal("temporary files left")
	}
}
func TestLimits(t *testing.T) {
	p, _, _ := fixture(t)
	m := Manifest{Version: 1, Coverage: Coverage, Subject: p.Subject, PlanDigest: canonical.BytesDigest(nil)}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		m.Members = append(m.Members, Entry{ID: id, Kind: "source", Size: MaxFile, Digest: canonical.BytesDigest(nil)})
	}
	if m.Validate() == nil {
		t.Fatal("total size overflow accepted")
	}
}
