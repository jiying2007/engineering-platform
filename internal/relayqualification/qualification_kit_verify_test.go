package relayqualification

import (
	"context"
	"testing"
)

func TestVerifyQualificationKitAcceptsExactBuiltKit(t *testing.T) {
	root, executable := runtimeTestExecutable(t)
	_ = root
	contract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", contract.RequestedModel)
	kit, err := BuildQualificationKit(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := VerifyQualificationKit(kit)
	if err != nil {
		t.Fatal(err)
	}
	if verification.ManifestDigest != kit.ManifestDigest ||
		verification.ProviderConfigDigest == "" ||
		verification.LiveManifestDigest == "" ||
		verification.RuntimeHandoffDigest == "" ||
		verification.CodexQualificationDigest == "" ||
		verification.CodexBinaryDigest != qualification.BinaryDigest ||
		verification.RequestedModel != contract.RequestedModel ||
		verification.FilesVerified != 7 {
		t.Fatalf("unexpected verification: %#v", verification)
	}
}

func TestVerifyQualificationKitRejectsAnyRetainedByteDrift(t *testing.T) {
	_, executable := runtimeTestExecutable(t)
	contract := validContract()
	qualification := runtimeQualificationFixture(t, executable, "0.157.1", contract.RequestedModel)
	base, err := BuildQualificationKit(context.Background(), contract, executable, qualification)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*QualificationKit)
	}{
		{"contract", func(k *QualificationKit) { k.ContractJSON[0] ^= 1 }},
		{"config", func(k *QualificationKit) { k.CodexConfig = append([]byte{}, k.CodexConfig...); k.CodexConfig[0] ^= 1 }},
		{"live", func(k *QualificationKit) {
			k.LiveManifestJSON = append([]byte{}, k.LiveManifestJSON...)
			k.LiveManifestJSON[0] ^= 1
		}},
		{"handoff", func(k *QualificationKit) {
			k.RuntimeHandoffJSON = append([]byte{}, k.RuntimeHandoffJSON...)
			k.RuntimeHandoffJSON[0] ^= 1
		}},
		{"qualification", func(k *QualificationKit) {
			k.QualificationJSON = append([]byte{}, k.QualificationJSON...)
			k.QualificationJSON[0] ^= 1
		}},
		{"manifest", func(k *QualificationKit) {
			k.ManifestJSON = append([]byte{}, k.ManifestJSON...)
			k.ManifestJSON[0] ^= 1
		}},
		{"checksums", func(k *QualificationKit) { k.Checksums = append([]byte{}, k.Checksums...); k.Checksums[0] ^= 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kit := QualificationKit{
				ContractJSON:       append([]byte{}, base.ContractJSON...),
				CodexConfig:        append([]byte{}, base.CodexConfig...),
				LiveManifestJSON:   append([]byte{}, base.LiveManifestJSON...),
				RuntimeHandoffJSON: append([]byte{}, base.RuntimeHandoffJSON...),
				QualificationJSON:  append([]byte{}, base.QualificationJSON...),
				ManifestJSON:       append([]byte{}, base.ManifestJSON...),
				Checksums:          append([]byte{}, base.Checksums...),
				ManifestDigest:     base.ManifestDigest,
			}
			tc.mutate(&kit)
			if _, err := VerifyQualificationKit(kit); err == nil {
				t.Fatal("drifted qualification kit accepted")
			}
		})
	}
}
