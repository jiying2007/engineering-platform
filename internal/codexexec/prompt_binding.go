package codexexec

import (
	"fmt"
	"unicode/utf8"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// VerifyTurnPromptBinding links the frozen Task/Preparation prompt identity,
// the exact text handed to the provider adapter and the resulting Codex
// EngineeringReceipt.PromptDigest. The bundle path is a host-local locator,
// so the stored identity stays locator-independent. Never recompute historic
// prompt text using a new catalog just to validate an archived receipt.
//
// Call before allowing a freshly executed turn to finalize/deliver. It proves
// bytes match the Worker-to-adapter input, not that the model obeyed them.
func VerifyTurnPromptBinding(
	assignment workerqueue.Assignment,
	prep preparation.Receipt,
	rendered string,
	promptIdentityDigest string,
	observedPromptDigest string,
) error {
	if len(rendered) == 0 || len(rendered) > 64<<10 || !utf8.ValidString(rendered) ||
		!canonical.ValidDigest(observedPromptDigest) {
		return fmt.Errorf("%w: bounded rendered Codex prompt and observed digest required", workerqueue.ErrIdentity)
	}
	expectedIdentity, err := PromptIdentityDigest(assignment, prep)
	if err != nil || promptIdentityDigest != expectedIdentity ||
		observedPromptDigest != canonical.BytesDigest([]byte(rendered)) {
		return fmt.Errorf("%w: Codex turn prompt bytes/identity disagree with frozen Task", workerqueue.ErrIdentity)
	}
	return nil
}
