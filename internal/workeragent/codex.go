package workeragent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

type CodexRuntime struct {
	Executable        string
	FederationRuleID  string
	IdentityTokenFile string
	SavedLoginFile    string
	AuditContext      string
}

func ExecuteCodex(ctx context.Context, c Transport, p *preparation.Preparer, request codexexec.Start, runtime CodexRuntime) (receipt codexexec.Receipt, err error) {
	if c == nil || p == nil || c.Subject() != p.Subject() || request.Profile.Validate() != nil ||
		runtime.Executable == "" {
		return receipt, fmt.Errorf("authenticated preparation owner and exact Codex profile/runtime required")
	}
	switch request.Profile.Provider.CredentialMode {
	case provideridentity.CredentialWorkloadIdentity:
		if runtime.FederationRuleID == "" || runtime.IdentityTokenFile == "" || runtime.SavedLoginFile != "" {
			return receipt, fmt.Errorf("complete workload-identity Codex runtime required")
		}
	case provideridentity.CredentialChatGPTSession:
		if runtime.SavedLoginFile == "" || runtime.FederationRuleID != "" || runtime.IdentityTokenFile != "" {
			return receipt, fmt.Errorf("isolated saved ChatGPT login Codex runtime required")
		}
	default:
		return receipt, fmt.Errorf("unsupported Codex provider credential mode")
	}
	var permit codexexec.Permit
	if err = c.Call(ctx, http.MethodPost, "/api/v1/worker/codex/start", request, &permit); err != nil {
		return receipt, err
	}
	if err = permit.Check(c.Subject(), request); err != nil {
		return receipt, err
	}
	finished := false
	defer func() {
		if !finished {
			cleanup, stop := context.WithTimeout(context.Background(), 6*time.Second)
			defer stop()
			_ = c.Call(cleanup, http.MethodPost, "/api/v1/worker/codex/fail", permit.Token, nil)
		}
	}()

	running, cancel := context.WithTimeout(ctx, 14*time.Minute)
	defer cancel()
	runCtx, stopRun := context.WithCancelCause(running)
	defer stopRun(nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
			renewal, stop := context.WithTimeout(runCtx, 6*time.Second)
			var response struct {
				LeaseUntil time.Time `json:"lease_until"`
			}
			failure := c.Call(renewal, http.MethodPost, "/api/v1/worker/codex/renew", permit.Token, &response)
			stop()
			if failure != nil || response.LeaseUntil.IsZero() {
				if failure == nil {
					failure = fmt.Errorf("empty Codex execution lease")
				}
				stopRun(failure)
				return
			}
		}
	}()
	joined := false
	defer func() {
		if !joined {
			stopRun(nil)
			<-done
		}
	}()

	prepared, err := p.Reopen(runCtx, c.Subject(), permit.Assignment, permit.Preparation.FactsDigest)
	if err != nil {
		return receipt, err
	}
	if err = p.SaveCodex(permit.Assignment, prepared, permit.Token.ID, permit); err != nil {
		return receipt, err
	}
	prompt, promptIdentityDigest, err := codexexec.Prompt(permit.Assignment, permit.Preparation, prepared.BundlePath)
	if err != nil {
		return receipt, err
	}
	codexReceipt, transcript, err := RunCodexTurn(runCtx, c, permit, runtime, prepared.Workspace.WorktreePath, prepared.Workspace.HomePath, prompt)
	controlDigest, controlErr := transcript.Digest()
	if controlErr == nil && transcript.Close.Binding.Token == permit.Token &&
		transcript.Close.Binding.ExecutionEpoch == permit.Assignment.Intent.ExecutionEpoch {
		saveErr := p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":control-transcript"))[7:], transcript)
		err = errors.Join(err, saveErr)
	} else if err == nil {
		if controlErr != nil {
			err = fmt.Errorf("sealed control transcript required: %w", controlErr)
		} else {
			err = fmt.Errorf("sealed control transcript binding mismatch")
		}
	}
	if err != nil {
		return receipt, retainStoppedSource(c, p, permit, prepared, transcript, err)
	}
	phase, expectedResult := "TURN_RECEIPT", ""
	finalizeEntered := false
	// RunCodexTurn already sealed actual process quiescence. Any later failure
	// preserves source without granting continuation or changing a Core receipt.
	defer func() {
		if err != nil {
			if !joined {
				stopRun(nil)
				<-done
				joined = true
			}
			receipt = codexexec.Receipt{}
			err = retainPostTurnFailure(c, p, permit, prepared, transcript, phase, expectedResult, finalizeEntered, err)
		}
	}()
	phaseStart := func(next string) error {
		phase = next
		return savePostTurnPhase(p, permit, prepared, transcript, phase, expectedResult)
	}
	if err = phaseStart("TURN_RECEIPT"); err != nil {
		return receipt, err
	}
	if err = p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":turn"))[7:], codexReceipt); err != nil {
		return receipt, err
	}
	if cause := context.Cause(runCtx); cause != nil {
		return receipt, cause
	}
	if err = phaseStart("FINALIZE"); err != nil {
		return receipt, err
	}
	finalizeEntered = true
	finalized, err := p.FinalizeChangedWorkspace(runCtx, c.Subject(), permit.Assignment, prepared, permit.Preparation.FactsDigest, permit.Token.ID)
	if err != nil {
		return receipt, err
	}
	if !canonical.ValidDigest(controlDigest) || !transcript.AllowsDelivery() {
		return receipt, fmt.Errorf("control transcript not eligible for delivery")
	}
	result := codexexec.Result{ControlTranscriptDigest: controlDigest, PromptIdentityDigest: promptIdentityDigest, Codex: codexReceipt, Change: finalized.Facts}
	if err = result.Validate(request.Profile, permit); err != nil {
		return receipt, err
	}
	expectedResult, err = canonical.Digest(result)
	if err != nil {
		return receipt, err
	}
	if err = phaseStart("RESULT_PERSIST"); err != nil {
		return receipt, err
	}
	local := struct {
		Result     codexexec.Result `json:"result"`
		BundlePath string           `json:"bundle_path"`
	}{Result: result, BundlePath: finalized.BundlePath}
	if err = p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":result"))[7:], local); err != nil {
		return receipt, err
	}
	stopRun(nil)
	<-done
	joined = true
	if err = phaseStart("REPORT_RENEW"); err != nil {
		return receipt, err
	}
	var renewal struct {
		LeaseUntil time.Time `json:"lease_until"`
	}
	if err = c.Call(ctx, http.MethodPost, "/api/v1/worker/codex/renew", permit.Token, &renewal); err != nil {
		return receipt, err
	}
	if renewal.LeaseUntil.IsZero() {
		return receipt, fmt.Errorf("empty final Codex execution lease")
	}
	if err = phaseStart("RESULT_REPORT"); err != nil {
		return receipt, err
	}
	report := codexexec.Report{Token: permit.Token, Result: result}
	if err = c.Call(ctx, http.MethodPost, "/api/v1/worker/codex/report", report, &receipt); err != nil && ctx.Err() == nil {
		var rejected *controlclient.HTTPError
		if !errors.As(err, &rejected) || rejected.Status >= 500 {
			err = c.Call(ctx, http.MethodPost, "/api/v1/worker/codex/report", report, &receipt)
		}
	}
	if err != nil {
		return receipt, err
	}
	if err = phaseStart("RECEIPT_VERIFY"); err != nil {
		return receipt, err
	}
	if err = receipt.Verify(c.Subject(), permit, result); err != nil {
		return receipt, err
	}
	finished = true
	return receipt, nil
}
