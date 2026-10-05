package sandbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"regexp"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// Build outputs are a deliberately small, explicit extension of the existing
// offline profile. They never require a writable host mount or a new authority.
// Their bytes share the existing bounded local/remote execution receipt.
const BuildOutputDirectory = "/tmp/ep-output"
const BuildOutputLimit = 256 << 10
const BuildFileLimit = 128 << 10
const BuildMemberLimit = 8
const BuildWireLimit = 768 << 10
const BuildGuardMode = "--build-output-v1"

var buildName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type OutputSpec struct {
	Name     string `json:"name"`
	MaxBytes int    `json:"max_bytes"`
}

// No path expansion, optional member, glob or late output exclusion is allowed.
// Every declared name is a required regular leaf in BuildOutputDirectory.
func ValidateOutputContract(contract []OutputSpec) error {
	if len(contract) < 1 || len(contract) > BuildMemberLimit {
		return ErrPolicy
	}
	total, previous := 0, ""
	for _, spec := range contract {
		if !buildName.MatchString(spec.Name) || spec.Name <= previous || spec.MaxBytes < 1 || spec.MaxBytes > BuildFileLimit {
			return ErrPolicy
		}
		total += spec.MaxBytes
		previous = spec.Name
	}
	if total > BuildOutputLimit {
		return ErrPolicy
	}
	return nil
}

func OutputContractDigest(contract []OutputSpec) (string, error) {
	if err := ValidateOutputContract(contract); err != nil {
		return "", err
	}
	raw, err := json.Marshal(contract)
	return Hash(raw), err
}

type OutputFile struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	Digest string `json:"digest"`
	Bytes  []byte `json:"bytes"`
}

type BuildOutputs struct {
	Contract       []OutputSpec `json:"contract"`
	ContractDigest string       `json:"contract_digest"`
	State          string       `json:"state"`
	ChildrenReaped bool         `json:"children_reaped"`
	Files          []OutputFile `json:"files"`
}

// Validate checks self-contained recorded bytes. The live Core additionally
// compares Contract to the exact Profile frozen in the existing permit.
func (b *BuildOutputs) Validate(exitCode int) error {
	if b == nil {
		return nil
	}
	digest, err := OutputContractDigest(b.Contract)
	if err != nil || digest != b.ContractDigest || !b.ChildrenReaped || exitCode < 0 || exitCode > 255 {
		return ErrPolicy
	}
	if exitCode != 0 {
		if b.State != "NOT_COLLECTED_EXIT_NONZERO" || len(b.Files) != 0 {
			return ErrPolicy
		}
		return nil
	}
	if b.State != "COLLECTED" || len(b.Files) != len(b.Contract) {
		return ErrPolicy
	}
	for i, f := range b.Files {
		spec := b.Contract[i]
		if f.Name != spec.Name || f.Size < 0 || f.Size > spec.MaxBytes || len(f.Bytes) != f.Size || f.Digest != Hash(f.Bytes) {
			return ErrPolicy
		}
	}
	return nil
}

// GuardArguments is also used by the independent container-inspection check.
func GuardArguments(p Profile) []string {
	args := []string{}
	if len(p.Outputs) != 0 {
		raw, _ := json.Marshal(p.Outputs) // caller already validated the Profile
		args = append(args, BuildGuardMode, base64.StdEncoding.EncodeToString(raw), "--")
	}
	return append(args, p.Argv...)
}

func DecodeOutputContract(encoded string) ([]OutputSpec, error) {
	if len(encoded) > 4096 {
		return nil, ErrPolicy
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != encoded {
		return nil, ErrPolicy
	}
	var wrapper struct {
		Outputs []OutputSpec `json:"outputs"`
	}
	wrapped := append([]byte(`{"outputs":`), raw...)
	wrapped = append(wrapped, '}')
	if strictjson.Decode(wrapped, &wrapper) != nil || ValidateOutputContract(wrapper.Outputs) != nil {
		return nil, ErrPolicy
	}
	contract := wrapper.Outputs
	exact, err := json.Marshal(contract)
	if err != nil || !bytes.Equal(exact, raw) {
		return nil, ErrPolicy
	}
	return contract, nil
}

// Only the pinned PID1 guard emits this envelope, after collecting command pipes
// and reaping all children. The host treats its payloads as untrusted build data.
// Direct child writes to the container log cannot be combined with a genuine
// envelope: there must be exactly one canonical document and no outer stderr.
// This is a bounded local-engine attestation, not a signed build certification.
type BuildEnvelope struct {
	Version  int          `json:"version"`
	ExitCode int          `json:"exit_code"`
	Stdout   []byte       `json:"stdout"`
	Stderr   []byte       `json:"stderr"`
	Outputs  BuildOutputs `json:"outputs"`
}

func (e BuildEnvelope) Validate() error {
	if e.Version != 1 || e.ExitCode == 122 || len(e.Stdout) > OutputLimit || len(e.Stderr) > OutputLimit {
		return ErrPolicy
	}
	return e.Outputs.Validate(e.ExitCode)
}

func EncodeBuildEnvelope(e BuildEnvelope) ([]byte, error) {
	if e.Validate() != nil {
		return nil, ErrPolicy
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw)+1 > BuildWireLimit {
		return nil, ErrPolicy
	}
	return append(raw, '\n'), nil
}

func DecodeBuildEnvelope(raw []byte, p Profile, exitCode int) (BuildEnvelope, error) {
	var e BuildEnvelope
	if len(raw) > BuildWireLimit || p.Validate() != nil || len(p.Outputs) == 0 || strictjson.Decode(raw, &e) != nil || e.Validate() != nil || e.ExitCode != exitCode || !reflect.DeepEqual(e.Outputs.Contract, p.Outputs) {
		return BuildEnvelope{}, ErrPolicy
	}
	exact, err := EncodeBuildEnvelope(e)
	if err != nil || !bytes.Equal(raw, exact) {
		return BuildEnvelope{}, ErrPolicy
	}
	return e, nil
}
