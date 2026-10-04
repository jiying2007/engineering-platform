package workeragent

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

type codexController struct {
	transport  Transport
	binding    codexexec.ControlBinding
	transcript codexexec.ControlTranscript
}

var _ codexapp.EngineeringController = (*codexController)(nil)

func (c *codexController) Bind(ctx context.Context, thread, turn string) error {
	c.binding.ThreadID = thread
	c.binding.TurnID = turn
	if c.binding.Validate() != nil {
		return fmt.Errorf("invalid runtime control binding")
	}
	var response struct {
		Bound bool `json:"bound"`
	}
	if err := c.transport.Call(ctx, http.MethodPost, "/api/v1/worker/codex/controls/bind", c.binding, &response); err != nil {
		return err
	}
	if !response.Bound {
		return fmt.Errorf("Core did not bind active Runtime")
	}
	return nil
}
func (c *codexController) Claim(ctx context.Context) (*codexapp.LiveControl, error) {
	var response struct {
		Delivery *codexexec.ControlDelivery `json:"delivery"`
	}
	if err := c.transport.Call(ctx, http.MethodPost, "/api/v1/worker/codex/controls/claim", c.binding, &response); err != nil {
		return nil, err
	}
	d := response.Delivery
	if d == nil {
		return nil, nil
	}
	if d.Validate() != nil || d.Payload.Binding != c.binding || d.State != codexexec.ControlDispatching {
		return nil, fmt.Errorf("Core control claim mismatch")
	}
	return &codexapp.LiveControl{ID: d.Command.ID, Kind: d.Payload.Kind, Text: d.Payload.Text, ThreadID: c.binding.ThreadID, TurnID: c.binding.TurnID}, nil
}
func (c *codexController) Report(ctx context.Context, id, outcome string) error {
	request := codexexec.ControlSettlement{Binding: c.binding, ID: id, Outcome: outcome}
	var d codexexec.ControlDelivery
	if err := c.transport.Call(ctx, http.MethodPost, "/api/v1/worker/codex/controls/report", request, &d); err != nil {
		return err
	}
	if d.Validate() != nil || d.Command.ID != id || d.Payload.Binding != c.binding || d.State != outcome {
		return fmt.Errorf("Core control outcome mismatch")
	}
	return nil
}
func (c *codexController) Close(ctx context.Context, status string, proof processscope.Proof) error {
	request := codexexec.ControlClose{Binding: c.binding, TurnStatus: status, ProcessScope: proof}
	var transcript codexexec.ControlTranscript
	if err := c.transport.Call(ctx, http.MethodPost, "/api/v1/worker/codex/controls/close", request, &transcript); err != nil {
		return err
	}
	if _, err := transcript.Digest(); err != nil || transcript.Close != request {
		return fmt.Errorf("Core control transcript mismatch")
	}
	c.transcript = transcript
	if status == "completed" && !transcript.AllowsDelivery() {
		return fmt.Errorf("unresolved or interrupted controls block delivery")
	}
	return nil
}

// RunCodexTurn is the shared Core-bound model-turn segment of ExecuteCodex.
// Callers must supply the prepared workspace; this does not mint a permit,
// relax sandbox policy, publish changes, or retry a model/control request.
func RunCodexTurn(ctx context.Context, c Transport, p codexexec.Permit, r CodexRuntime, work, home, prompt string) (codexapp.EngineeringReceipt, codexexec.ControlTranscript, error) {
	var receipt codexapp.EngineeringReceipt
	var transcript codexexec.ControlTranscript
	if c == nil || p.Check(c.Subject(), codexexec.Start{RunID: p.Token.RunID, WorkerProfile: p.Token.WorkerProfile, Profile: p.Profile}) != nil {
		return receipt, transcript, fmt.Errorf("exact Core permit required")
	}
	controller := &codexController{transport: c, binding: codexexec.ControlBinding{Token: p.Token, ExecutionEpoch: p.Assignment.Intent.ExecutionEpoch}}
	var err error
	switch p.Profile.Provider.CredentialMode {
	case provideridentity.CredentialWorkloadIdentity:
		receipt, err = codexapp.EngineeringWIFTurn(ctx, r.Executable, p.Profile.CodexVersion, p.Profile.BinaryDigest, p.Profile.QualificationDigest, work, home, r.FederationRuleID, r.IdentityTokenFile, r.AuditContext, p.Profile.Model, prompt, controller)
	case provideridentity.CredentialChatGPTSession:
		receipt, err = codexapp.EngineeringSavedLoginTurn(ctx, r.Executable, p.Profile.CodexVersion, p.Profile.BinaryDigest, p.Profile.QualificationDigest, work, home, r.SavedLoginFile, p.Profile.Model, prompt, controller)
	default:
		err = fmt.Errorf("unsupported qualified credential lane")
	}
	return receipt, controller.transcript, err
}
