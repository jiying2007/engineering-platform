package workeragent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

type CodexRuntime struct {
	Executable        string
	FederationRuleID  string
	IdentityTokenFile string
	AuditContext      string
}

func ExecuteCodex(ctx context.Context, c Transport, p *preparation.Preparer, request codexexec.Start, runtime CodexRuntime) (receipt codexexec.Receipt, err error) {
	if c == nil || p == nil || c.Subject() != p.Subject() || request.Profile.Validate() != nil ||
		runtime.Executable == "" || runtime.FederationRuleID == "" || runtime.IdentityTokenFile == "" {
		return receipt, fmt.Errorf("authenticated preparation owner, exact Codex profile and WIF runtime required")
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
	codexReceipt, err := codexapp.EngineeringWIFTurn(
		runCtx,
		runtime.Executable,
		request.Profile.BinaryDigest,
		prepared.Workspace.WorktreePath,
		prepared.Workspace.HomePath,
		runtime.FederationRuleID,
		runtime.IdentityTokenFile,
		runtime.AuditContext,
		request.Profile.Model,
		prompt,
	)
	if err != nil {
		return receipt, err
	}
	if err = p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":turn"))[7:], codexReceipt); err != nil {
		return receipt, err
	}
	if cause := context.Cause(runCtx); cause != nil {
		return receipt, cause
	}
	finalized, err := p.FinalizeChangedWorkspace(runCtx, c.Subject(), permit.Assignment, prepared, permit.Preparation.FactsDigest, permit.Token.ID)
	if err != nil {
		return receipt, err
	}
	result := codexexec.Result{PromptIdentityDigest: promptIdentityDigest, Codex: codexReceipt, Change: finalized.Facts}
	if err = result.Validate(request.Profile, permit); err != nil {
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
	var renewal struct {
		LeaseUntil time.Time `json:"lease_until"`
	}
	if err = c.Call(ctx, http.MethodPost, "/api/v1/worker/codex/renew", permit.Token, &renewal); err != nil {
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
	if err = receipt.Verify(c.Subject(), permit, result); err != nil {
		return receipt, err
	}
	finished = true
	return receipt, nil
}
