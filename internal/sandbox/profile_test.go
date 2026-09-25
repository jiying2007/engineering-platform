package sandbox

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func fixtureProfile() Profile {
	return Profile{Image: Hash([]byte("image")), GuardDigest: Hash([]byte("guard")), Argv: []string{"/probe"}, Seconds: 3}
}
func TestProfileAndOutputIdentity(t *testing.T) {
	p := fixtureProfile()
	d, err := p.Digest()
	if err != nil {
		t.Fatal(err)
	}
	changed := p
	changed.Argv = []string{"/different"}
	other, _ := changed.Digest()
	if other == d {
		t.Fatal("command not bound")
	}
	for _, mutate := range []func(*Profile){func(p *Profile) { p.Image = "alpine:latest" }, func(p *Profile) { p.GuardDigest = "" }, func(p *Profile) { p.Seconds = 46 }, func(p *Profile) { p.Argv = []string{"relative"} }, func(p *Profile) { p.Argv = []string{"/ep-guard"} }, func(p *Profile) { p.Argv = []string{"/probe", "bad\x00value"} }} {
		v := p
		mutate(&v)
		if v.Validate() == nil {
			t.Fatalf("invalid profile: %#v", v)
		}
	}
	r := Result{Recipe: Recipe, ContainerID: strings.Repeat("a", 64), ProfileDigest: d, UserID: 1000, Stdout: []byte("real bytes"), Stderr: []byte{}, StdoutDigest: Hash([]byte("real bytes")), StderrDigest: Hash(nil)}
	if r.Validate(p) != nil {
		t.Fatal("valid result denied")
	}
	r.Stdout = []byte("tampered")
	if r.Validate(p) == nil {
		t.Fatal("tamper accepted")
	}
}
func TestBoundedDockerLogFraming(t *testing.T) {
	frame := func(stream byte, data string) []byte {
		buf := make([]byte, 8)
		buf[0] = stream
		binary.BigEndian.PutUint32(buf[4:], uint32(len(data)))
		return append(buf, []byte(data)...)
	}
	a, b, err := demultiplex(append(frame(1, "hello"), frame(2, "error")...))
	if err != nil || string(a) != "hello" || string(b) != "error" {
		t.Fatal("bad demux")
	}
	for _, data := range [][]byte{{1, 0}, frame(3, "bad"), frame(1, strings.Repeat("x", OutputLimit+1)), frame(1, "abc")[:9]} {
		if _, _, err := demultiplex(data); err == nil {
			t.Fatal("malformed/oversized log accepted")
		}
	}
}
func TestInspectRejectsMissingConstraints(t *testing.T) {
	p := fixtureProfile()
	c := map[string]any{"Image": p.Image, "Config": map[string]any{"User": "1000:1000", "WorkingDir": "/workspace", "Labels": map[string]string{"engineering-platform.offline": "owner"}, "Entrypoint": []string{"/ep-guard"}, "Cmd": []string{"3", "/probe"}, "Env": []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "LANG=C", "TMPDIR=/tmp"}}, "HostConfig": map[string]any{"NetworkMode": "none", "IpcMode": "private", "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Memory": 256 << 20, "MemorySwap": 256 << 20, "NanoCpus": 1_000_000_000, "PidsLimit": 64, "RestartPolicy": map[string]string{"Name": "no"}, "LogConfig": map[string]string{"Type": "local"}, "Tmpfs": map[string]string{"/tmp": "rw,nosuid,nodev,noexec,size=67108864,mode=1777"}}, "Mounts": []any{map[string]any{"Type": "bind", "Source": "/source", "Destination": "/workspace", "Propagation": "rprivate"}, map[string]any{"Type": "bind", "Source": "/bundle", "Destination": "/context", "Propagation": "rprivate"}, map[string]any{"Type": "bind", "Source": "/guard", "Destination": "/ep-guard", "Propagation": "rprivate"}}}
	raw, _ := json.Marshal(c)
	if err := validateContainer(raw, p, "1000:1000", "owner", "/source", "/bundle", "/guard"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ReadonlyRootfs", "Memory", "MemorySwap", "NanoCpus", "PidsLimit", "SecurityOpt", "CapDrop"} {
		copy := map[string]any{}
		_ = json.Unmarshal(raw, &copy)
		delete(copy["HostConfig"].(map[string]any), field)
		b, _ := json.Marshal(copy)
		if validateContainer(b, p, "1000:1000", "owner", "/source", "/bundle", "/guard") == nil {
			t.Fatal("constraint ignored", field)
		}
	}
}
