package main

import (
	"os"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func runtimeQualificationFixtureForCLI(t *testing.T, executable, model string) codexapp.QualificationReceipt {
	t.Helper()
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	return codexapp.QualificationReceipt{
		SchemaVersion:                3,
		CompatibilityContractVersion: codexapp.CompatibilityContractVersion,
		CLI:                          "codex-cli",
		Version:                      "0.157.1",
		BinaryDigest:                 canonical.BytesDigest(data),
		StableSchemaDigest:           canonical.BytesDigest([]byte("stable")),
		ExperimentalSchemaDigest:     canonical.BytesDigest([]byte("experimental")),
		Transport:                    "stdio",
		FreshProcess:                 true,
		ManagedDaemon:                false,
		PerThreadConfigOverride:      false,
		InitializePassed:             true,
		ThreadStartPassed:            true,
		ThreadStartModel:             model,
		StableSchemaContractChecked:  true,
		ExperimentalSurfaceChecked:   true,
		CredentialSafeConfigDigest:   codexapp.CredentialSafeConfigDigest(),
		CredentialSafeProfileChecked: true,
		EngineeringConfigDigest:      codexapp.EngineeringConfigDigest(),
		EngineeringProfileChecked:    true,
		IsolationMechanism:           "linux-user-pid-namespace-init-v1",
		IsolationEnvironmentDigest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		IsolatedEngineeringStartup:   true,
		NamespaceInitReaped:          true,
	}
}
