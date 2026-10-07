package codexapp

import (
	"encoding/json"
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const (
	MaxEngineeringHistoryItems    = 2048
	MaxEngineeringHistoryRawBytes = 2 << 20
)

// EngineeringHistory is private reconciliation material. Params are exact
// provider-owned JSON bytes; []byte encoding keeps foreign camelCase keys opaque
// to the platform's own strict lower_snake_case record schema.
type EngineeringHistory struct {
	Version    int      `json:"version"`
	ThreadID   string   `json:"thread_id"`
	TurnID     string   `json:"turn_id"`
	Items      [][]byte `json:"item_completed_params"`
	Completion []byte   `json:"turn_completed_params"`
}

func engineeringHistoryItem(raw []byte, threadID, turnID string) error {
	if len(raw) == 0 || len(raw) > strictjson.MaxBytes || strictjson.ValidateForeignObject(raw) != nil {
		return fmt.Errorf("invalid engineering item history bytes")
	}
	var p struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Item     struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &p) != nil || p.ThreadID != threadID || p.TurnID != turnID || !remoteID(p.Item.ID) {
		return fmt.Errorf("engineering item history identity mismatch")
	}
	switch p.Item.Type {
	case "agentMessage", "commandExecution", "fileChange", "userMessage", "reasoning", "plan":
		return nil
	default:
		return fmt.Errorf("disallowed engineering item history type")
	}
}

func engineeringHistoryCompletion(raw []byte, threadID, turnID string) error {
	if len(raw) == 0 || len(raw) > strictjson.MaxBytes || strictjson.ValidateForeignObject(raw) != nil {
		return fmt.Errorf("invalid engineering completion history bytes")
	}
	var p struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	if json.Unmarshal(raw, &p) != nil || p.ThreadID != threadID || p.Turn.ID != turnID {
		return fmt.Errorf("engineering completion history identity mismatch")
	}
	switch p.Turn.Status {
	case "completed", "interrupted", "failed":
		return nil
	default:
		return fmt.Errorf("invalid engineering completion history status")
	}
}

func (h EngineeringHistory) Validate() error {
	if h.Version != 1 || !remoteID(h.ThreadID) || !remoteID(h.TurnID) ||
		h.Items == nil || len(h.Items) > MaxEngineeringHistoryItems {
		return fmt.Errorf("invalid engineering history identity or item bound")
	}
	total := len(h.Completion)
	for _, raw := range h.Items {
		total += len(raw)
		if total > MaxEngineeringHistoryRawBytes || engineeringHistoryItem(raw, h.ThreadID, h.TurnID) != nil {
			return fmt.Errorf("invalid or oversized engineering item history")
		}
	}
	if total > MaxEngineeringHistoryRawBytes ||
		engineeringHistoryCompletion(h.Completion, h.ThreadID, h.TurnID) != nil {
		return fmt.Errorf("invalid or oversized engineering completion history")
	}
	return nil
}

func (h EngineeringHistory) Digest() (string, error) {
	if err := h.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(h)
}

func (h *EngineeringHistory) appendItem(raw []byte) error {
	if h == nil || h.Completion != nil || len(h.Items) >= MaxEngineeringHistoryItems ||
		engineeringHistoryItem(raw, h.ThreadID, h.TurnID) != nil {
		return fmt.Errorf("invalid engineering item history")
	}
	total := len(raw) + len(h.Completion)
	for _, item := range h.Items {
		total += len(item)
	}
	if total > MaxEngineeringHistoryRawBytes {
		return fmt.Errorf("engineering item history exceeds bound")
	}
	h.Items = append(h.Items, append([]byte(nil), raw...))
	return nil
}

func (h *EngineeringHistory) complete(raw []byte) error {
	if h == nil || h.Completion != nil || engineeringHistoryCompletion(raw, h.ThreadID, h.TurnID) != nil {
		return fmt.Errorf("invalid engineering completion history")
	}
	total := len(raw)
	for _, item := range h.Items {
		total += len(item)
	}
	if total > MaxEngineeringHistoryRawBytes {
		return fmt.Errorf("engineering history exceeds bound")
	}
	h.Completion = append([]byte(nil), raw...)
	return h.Validate()
}
