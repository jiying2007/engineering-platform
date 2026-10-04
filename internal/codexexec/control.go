package codexexec

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/session"
)

const (
	ControlSteer        = "STEER"
	ControlInterrupt    = "INTERRUPT"
	ControlQueued       = "QUEUED"
	ControlDispatching  = "DISPATCHING"
	ControlAccepted     = "STEER_ACCEPTED"
	ControlInterruptACK = "INTERRUPT_ACKNOWLEDGED"
	ControlInterrupted  = "TURN_INTERRUPTED"
	ControlNotApplied   = "NOT_APPLIED"
	ControlUnknown      = "UNKNOWN"
	MaxControls         = 64
)

// ControlInput uses the existing RunControl authority and session sequence.
// Text is extra user input, never a new Task, provider, tool or approval grant.
type ControlInput struct {
	ID             string `json:"steering_command_id"`
	ExecutionEpoch uint64 `json:"execution_epoch"`
	Sequence       uint64 `json:"sequence"`
	Actor          string `json:"actor"`
	ThreadID       string `json:"thread_id"`
	TurnID         string `json:"expected_turn_id"`
	Text           string `json:"text,omitempty"`
}

func boundedID(s string) bool {
	if s == "" || len(s) > 256 || strings.TrimSpace(s) != s || !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}
func (i ControlInput) Validate(kind string) error {
	if !boundedID(i.ID) || !boundedID(i.Actor) || !boundedID(i.ThreadID) || !boundedID(i.TurnID) || i.ExecutionEpoch == 0 || i.ExecutionEpoch > 1<<62 || i.Sequence == 0 || i.Sequence > 1<<62 {
		return fmt.Errorf("invalid control identity")
	}
	switch kind {
	case ControlSteer:
		if strings.TrimSpace(i.Text) == "" || len(i.Text) > 8192 || !utf8.ValidString(i.Text) || strings.ContainsRune(i.Text, 0) {
			return fmt.Errorf("bounded nonempty UTF-8 steering text required")
		}
	case ControlInterrupt:
		if i.Text != "" {
			return fmt.Errorf("interrupt must not carry model input")
		}
	default:
		return fmt.Errorf("unsupported live control")
	}
	return nil
}

type ControlBinding struct {
	Token          Token  `json:"token"`
	ExecutionEpoch uint64 `json:"execution_epoch"`
	ThreadID       string `json:"thread_id"`
	TurnID         string `json:"turn_id"`
}

func (b ControlBinding) Validate() error {
	if !b.Token.Valid() || b.ExecutionEpoch == 0 || b.ExecutionEpoch > 1<<62 || !boundedID(b.ThreadID) || !boundedID(b.TurnID) {
		return fmt.Errorf("invalid live Runtime binding")
	}
	return nil
}

type ControlPayload struct {
	Binding ControlBinding `json:"binding"`
	Kind    string         `json:"kind"`
	Text    string         `json:"text,omitempty"`
}
type ControlDelivery struct {
	Command      session.SteeringCommand `json:"command"`
	Payload      ControlPayload          `json:"payload"`
	State        string                  `json:"delivery_state"`
	DispatchedAt *time.Time              `json:"dispatched_at,omitempty"`
	ResolvedAt   *time.Time              `json:"resolved_at,omitempty"`
}

func (d ControlDelivery) Validate() error {
	c, b := d.Command, d.Payload.Binding
	i := ControlInput{ID: c.ID, ExecutionEpoch: c.ExecutionEpoch, Sequence: c.Sequence, Actor: c.Actor, ThreadID: b.ThreadID, TurnID: b.TurnID, Text: d.Payload.Text}
	hash, err := canonical.Digest(d.Payload)
	if err != nil || b.Validate() != nil || i.Validate(d.Payload.Kind) != nil || c.RunID != b.Token.RunID || c.ExecutionEpoch != b.ExecutionEpoch || c.ContentDigest != hash || c.CreatedAt.IsZero() {
		return fmt.Errorf("control payload/authority mismatch")
	}
	if d.DispatchedAt != nil && (d.DispatchedAt.IsZero() || d.DispatchedAt.Before(c.CreatedAt)) {
		return fmt.Errorf("invalid dispatch observation")
	}
	if d.ResolvedAt != nil && (d.ResolvedAt.IsZero() || d.ResolvedAt.Before(c.CreatedAt) || (d.DispatchedAt != nil && d.ResolvedAt.Before(*d.DispatchedAt))) {
		return fmt.Errorf("invalid resolution observation")
	}
	switch d.State {
	case ControlQueued:
		if d.DispatchedAt != nil || d.ResolvedAt != nil {
			return fmt.Errorf("queued command carries outcome")
		}
	case ControlDispatching:
		if d.DispatchedAt == nil || d.ResolvedAt != nil {
			return fmt.Errorf("invalid dispatch state")
		}
	case ControlAccepted, ControlInterruptACK, ControlInterrupted, ControlUnknown:
		if d.DispatchedAt == nil || d.ResolvedAt == nil {
			return fmt.Errorf("missing delivery observations")
		}
		if (d.State == ControlAccepted && d.Payload.Kind != ControlSteer) || ((d.State == ControlInterruptACK || d.State == ControlInterrupted) && d.Payload.Kind != ControlInterrupt) {
			return fmt.Errorf("outcome kind mismatch")
		}
	case ControlNotApplied:
		if d.ResolvedAt == nil {
			return fmt.Errorf("missing not-applied observation")
		}
	default:
		return fmt.Errorf("invalid delivery outcome")
	}
	return nil
}

type ControlSettlement struct {
	Binding ControlBinding `json:"binding"`
	ID      string         `json:"steering_command_id"`
	Outcome string         `json:"outcome"`
}
type ControlClose struct {
	Binding    ControlBinding `json:"binding"`
	TurnStatus string         `json:"turn_status"`
	// This is only the app-server process, not proof of descendant quiescence.
	RuntimeExited bool `json:"runtime_process_exited"`
}

func (c ControlClose) Validate() error {
	if c.Binding.Validate() != nil {
		return fmt.Errorf("invalid closure binding")
	}
	switch c.TurnStatus {
	case "completed", "interrupted", "failed", "unknown":
		return nil
	}
	return fmt.Errorf("invalid terminal turn observation")
}

type ControlTranscript struct {
	Version    int               `json:"version"`
	Close      ControlClose      `json:"close"`
	Deliveries []ControlDelivery `json:"deliveries"`
}

func (t ControlTranscript) Digest() (string, error) {
	if t.Version != 1 || t.Close.Validate() != nil || len(t.Deliveries) > MaxControls {
		return "", fmt.Errorf("invalid control transcript")
	}
	var last uint64
	seen := map[string]bool{}
	for _, d := range t.Deliveries {
		if d.Validate() != nil || d.Payload.Binding != t.Close.Binding || d.Command.Sequence <= last || seen[d.Command.ID] || d.State == ControlQueued || d.State == ControlDispatching {
			return "", fmt.Errorf("control transcript drift")
		}
		last = d.Command.Sequence
		seen[d.Command.ID] = true
	}
	return canonical.Digest(t)
}
func (t ControlTranscript) AllowsDelivery() bool {
	if _, err := t.Digest(); err != nil || t.Close.TurnStatus != "completed" || !t.Close.RuntimeExited {
		return false
	}
	for _, d := range t.Deliveries {
		if d.Payload.Kind != ControlSteer || d.State != ControlAccepted {
			return false
		}
	}
	return true
}

type ControlRuntime struct {
	Binding    ControlBinding     `json:"binding"`
	State      string             `json:"state"`
	Transcript *ControlTranscript `json:"transcript,omitempty"`
	Digest     string             `json:"transcript_digest,omitempty"`
}
type ControlRepository interface {
	BindCodexControl(context.Context, string, ControlBinding) error
	QueueCodexControl(context.Context, string, string, ControlInput) (ControlDelivery, error)
	ClaimCodexControl(context.Context, string, ControlBinding) (*ControlDelivery, error)
	SettleCodexControl(context.Context, string, ControlSettlement) (ControlDelivery, error)
	CloseCodexControl(context.Context, string, ControlClose) (ControlTranscript, error)
	GetCodexControl(context.Context, string) (ControlDelivery, error)
	GetCodexControlRuntime(context.Context, string) (*ControlRuntime, error)
}

// VerifyFinishedControls is the common publication/evidence boundary. Historical
// receipts remain historical; a missing transcript never becomes live authority.
func (s Status) VerifyFinishedControls() error {
	if s.State != Finished || s.Receipt == nil || s.Receipt.Token != s.Token || s.Runtime == nil || s.Runtime.State != "SEALED" || s.Runtime.Transcript == nil {
		return fmt.Errorf("finished execution with sealed controls required")
	}
	t := s.Runtime.Transcript
	digest, err := t.Digest()
	if err != nil || !t.AllowsDelivery() || t.Close.Binding != s.Runtime.Binding || s.Runtime.Binding.Token != s.Token || s.Runtime.Binding.ThreadID != s.Receipt.Result.Codex.ThreadID || s.Runtime.Binding.TurnID != s.Receipt.Result.Codex.TurnID || digest != s.Runtime.Digest || digest != s.Receipt.Result.ControlTranscriptDigest {
		return fmt.Errorf("finished result/control transcript mismatch")
	}
	return nil
}
