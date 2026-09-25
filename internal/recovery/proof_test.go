package recovery

import (
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func validProof(t *testing.T) Proof {
	t.Helper()
	facts := Facts{}
	digest, err := canonical.Digest(facts)
	if err != nil {
		t.Fatal(err)
	}
	return Proof{
		Kind: ProofKind, RecoveryEpoch: 1, Reconciler: "urn:engineering-platform:operator:reconciler",
		Facts: facts, FactsDigest: digest, AuditSequence: 1,
		AuditDigest: "sha256:" + strings.Repeat("a", 64), CreatedAt: time.Unix(1, 0).UTC(),
	}
}

func TestProofValidation(t *testing.T) {
	p := validProof(t)
	if _, err := p.Digest(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"unresolved", "facts-digest", "audit", "epoch", "reconciler"} {
		t.Run(kind, func(t *testing.T) {
			bad := p
			switch kind {
			case "unresolved":
				bad.Facts.ExternalUnresolved = 1
			case "facts-digest":
				bad.FactsDigest = "sha256:" + strings.Repeat("b", 64)
			case "audit":
				bad.AuditDigest = "bad"
			case "epoch":
				bad.RecoveryEpoch = 0
			case "reconciler":
				bad.Reconciler = " reconciler "
			}
			if err := bad.Validate(); err == nil {
				t.Fatal("invalid proof accepted")
			}
		})
	}
}
