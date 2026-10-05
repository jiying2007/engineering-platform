package sandbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func buildFixture(t *testing.T) (Profile, BuildEnvelope) {
	t.Helper()
	p := fixtureProfile()
	p.Outputs = []OutputSpec{{Name: "firmware.bin", MaxBytes: 64}, {Name: "firmware.map", MaxBytes: 64}}
	d, err := OutputContractDigest(p.Outputs)
	if err != nil {
		t.Fatal(err)
	}
	e := BuildEnvelope{Version: 1, Outputs: BuildOutputs{Contract: p.Outputs, ContractDigest: d, State: "COLLECTED", ChildrenReaped: true, Files: []OutputFile{
		{Name: "firmware.bin", Size: 2, Digest: Hash([]byte{0, 255}), Bytes: []byte{0, 255}},
		{Name: "firmware.map", Size: 0, Digest: Hash(nil), Bytes: []byte{}},
	}}}
	return p, e
}
func TestBuildOutputFrozenContractAndReceipt(t *testing.T) {
	p, e := buildFixture(t)
	d, err := p.Digest()
	if err != nil {
		t.Fatal(err)
	}
	q := p
	q.Outputs = append([]OutputSpec{}, p.Outputs...)
	q.Outputs[0].MaxBytes++
	changed, _ := q.Digest()
	if d == changed {
		t.Fatal("output contract not frozen")
	}
	args := GuardArguments(p)
	decoded, err := DecodeOutputContract(args[1])
	if err != nil || len(decoded) != 2 || args[0] != BuildGuardMode || args[2] != "--" {
		t.Fatal("bad guard args", err)
	}
	raw, err := EncodeBuildEnvelope(e)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeBuildEnvelope(raw, p, 0)
	if err != nil || len(got.Outputs.Files) != 2 {
		t.Fatal("good envelope rejected", err)
	}
	r := Result{Recipe: Recipe, ProfileDigest: d, ContainerID: strings.Repeat("a", 64), UserID: 1000, StdoutDigest: Hash(nil), StderrDigest: Hash(nil), BuildOutputs: &got.Outputs}
	if err = r.Validate(p); err != nil {
		t.Fatal(err)
	}
	r.BuildOutputs = nil
	if r.Validate(p) == nil {
		t.Fatal("missing required outputs accepted")
	}
	r.BuildOutputs = &got.Outputs
	noOutput := fixtureProfile()
	r.ProfileDigest, _ = noOutput.Digest()
	if r.Validate(noOutput) == nil {
		t.Fatal("undeclared outputs accepted")
	}
	e.ExitCode = 7
	e.Outputs.State = "NOT_COLLECTED_EXIT_NONZERO"
	e.Outputs.Files = nil
	raw, err = EncodeBuildEnvelope(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeBuildEnvelope(raw, p, 7); err != nil {
		t.Fatal("nonzero exit facts rejected", err)
	}
	if _, err = DecodeBuildEnvelope(raw, p, 0); err == nil {
		t.Fatal("exit code drift accepted")
	}
}
func TestBuildOutputContractRejectsInvalidMembers(t *testing.T) {
	cases := [][]OutputSpec{nil, {}, {{Name: "../escape", MaxBytes: 1}}, {{Name: "a/b", MaxBytes: 1}}, {{Name: "a\\b", MaxBytes: 1}}, {{Name: ".hidden", MaxBytes: 1}}, {{Name: "a", MaxBytes: 0}}, {{Name: "a", MaxBytes: BuildFileLimit + 1}}, {{Name: "b", MaxBytes: 1}, {Name: "a", MaxBytes: 1}}, {{Name: "a", MaxBytes: 1}, {Name: "a", MaxBytes: 1}}, {{Name: "a", MaxBytes: BuildFileLimit}, {Name: "b", MaxBytes: BuildFileLimit}, {Name: "c", MaxBytes: 1}}}
	for i, c := range cases {
		if ValidateOutputContract(c) == nil {
			t.Fatalf("bad contract %d accepted", i)
		}
	}
	c := []OutputSpec{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		c = append(c, OutputSpec{Name: n, MaxBytes: 1})
	}
	if ValidateOutputContract(c) == nil {
		t.Fatal("excess members")
	}
	for _, raw := range []string{`[{"name":"a","name":"b","max_bytes":1}]`, ` [{"name":"a","max_bytes":1}]`, `[{"name":"a","max_bytes":1,"extra":1}]`} {
		if _, err := DecodeOutputContract(base64.StdEncoding.EncodeToString([]byte(raw))); err == nil {
			t.Fatal("invalid encoded contract")
		}
	}
}
func TestBuildEnvelopeRejectsTamperingAndNoncanonicalLogs(t *testing.T) {
	p, good := buildFixture(t)
	raw, _ := EncodeBuildEnvelope(good)
	for name, mutate := range map[string]func(*BuildEnvelope){
		"hash":    func(e *BuildEnvelope) { e.Outputs.Files[0].Digest = Hash(nil) },
		"size":    func(e *BuildEnvelope) { e.Outputs.Files[0].Size++ },
		"missing": func(e *BuildEnvelope) { e.Outputs.Files = e.Outputs.Files[:1] },
		"extra":   func(e *BuildEnvelope) { e.Outputs.Files = append(e.Outputs.Files, e.Outputs.Files[0]) },
		"name":    func(e *BuildEnvelope) { e.Outputs.Files[0].Name = "other" },
		"stop":    func(e *BuildEnvelope) { e.Outputs.ChildrenReaped = false },
		"state":   func(e *BuildEnvelope) { e.Outputs.State = "NOT_COLLECTED_EXIT_NONZERO" },
		"profile": func(e *BuildEnvelope) {
			e.Outputs.Contract[0].MaxBytes++
			e.Outputs.ContractDigest, _ = OutputContractDigest(e.Outputs.Contract)
		},
		"log-limit": func(e *BuildEnvelope) { e.Stdout = make([]byte, OutputLimit+1) },
	} {
		t.Run(name, func(t *testing.T) {
			var e BuildEnvelope
			if json.Unmarshal(raw, &e) != nil {
				t.Fatal("fixture")
			}
			mutate(&e)
			b, _ := json.Marshal(e)
			b = append(b, '\n')
			if _, err := DecodeBuildEnvelope(b, p, 0); err == nil {
				t.Fatal("corrupted output accepted")
			}
		})
	}
	for _, bad := range [][]byte{append([]byte("noise"), raw...), append(append([]byte{}, raw...), raw...), append(raw, '\n'), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), bytes.Repeat([]byte("x"), BuildWireLimit+1)} {
		if _, err := DecodeBuildEnvelope(bad, p, 0); err == nil {
			t.Fatal("noncanonical log accepted")
		}
	}
}
func TestBuildEnvelopeFitsExistingTransportAndLogBudgets(t *testing.T) {
	p, e := buildFixture(t)
	p.Outputs = []OutputSpec{{Name: "a", MaxBytes: BuildFileLimit}, {Name: "b", MaxBytes: BuildFileLimit}}
	e.Outputs.Contract = p.Outputs
	e.Outputs.ContractDigest, _ = OutputContractDigest(p.Outputs)
	e.Stdout = bytes.Repeat([]byte("s"), OutputLimit)
	e.Stderr = bytes.Repeat([]byte("e"), OutputLimit)
	for i := range e.Outputs.Files {
		b := bytes.Repeat([]byte{byte(i)}, BuildFileLimit)
		e.Outputs.Files[i] = OutputFile{Name: p.Outputs[i].Name, Size: len(b), Digest: Hash(b), Bytes: b}
	}
	raw, err := EncodeBuildEnvelope(e)
	if err != nil || len(raw) > BuildWireLimit {
		t.Fatal("bound mismatch", err, len(raw))
	}
	if _, err = DecodeBuildEnvelope(raw, p, 0); err != nil {
		t.Fatal(err)
	}
	r := Result{BuildOutputs: &e.Outputs, Stdout: e.Stdout, Stderr: e.Stderr}
	b, _ := json.Marshal(r)
	if len(b) > 900<<10 {
		t.Fatal("outputs no longer fit existing 1 MiB report budget")
	}
}
