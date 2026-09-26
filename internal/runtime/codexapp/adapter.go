package codexapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

var ErrLifecycle = errors.New("invalid app-server protocol lifecycle")

// Adapter maps one connection to one provider thread. It is NOT Core Run/epoch
// authority. The host must authorize each operation and persist exact identities.
// This first profile is read-only and never approves a server tool request.
type Adapter struct {
	client          *Client
	gate            chan struct{}
	mu              sync.Mutex
	initialized     bool
	thread          string
	turn            string
	worktree        string
	starting        bool
	earlyCompletion string
}

func NewAdapter(client *Client, worktree string) (*Adapter, error) {
	work, err := canonicalPath(worktree, true)
	if err != nil || client == nil {
		return nil, ErrLifecycle
	}
	a := &Adapter{client: client, worktree: work, gate: make(chan struct{}, 1)}
	a.gate <- struct{}{}
	return a, nil
}
func (a *Adapter) enter(ctx context.Context) error {
	if a == nil || a.client == nil {
		return ErrLifecycle
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.client.Done():
		return ErrClientClosed
	case <-a.gate:
		return nil
	}
}
func (a *Adapter) leave() { a.gate <- struct{}{} }
func (a *Adapter) Initialize(ctx context.Context, version string) error {
	if err := a.enter(ctx); err != nil {
		return err
	}
	defer a.leave()
	a.mu.Lock()
	ready := a.initialized
	a.mu.Unlock()
	if ready || strings.TrimSpace(version) == "" || len(version) > 128 {
		return ErrLifecycle
	}
	var result json.RawMessage
	err := a.client.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "engineering_platform", "version": version}, "capabilities": map[string]bool{"experimentalApi": false}}, &result)
	if err != nil {
		return err
	}
	if err = a.client.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return err
	}
	a.mu.Lock()
	a.initialized = true
	a.mu.Unlock()
	return nil
}
func (a *Adapter) WarmWorkloadIdentity(ctx context.Context) error {
	if err := a.enter(ctx); err != nil {
		return err
	}
	defer a.leave()
	a.mu.Lock()
	ready, thread, turn := a.initialized, a.thread, a.turn
	a.mu.Unlock()
	if !ready || thread != "" || turn != "" {
		return ErrLifecycle
	}
	// account/rateLimits/read resolves AuthManager::auth().await in Codex 0.155.0.
	// For workload identity this performs the assertion exchange without starting
	// a thread, model turn, tool, or approval flow. Success is the deletion fence:
	// the host may remove the upstream assertion before any model-reachable work.
	var result struct {
		RateLimits json.RawMessage `json:"rateLimits"`
	}
	if err := a.client.Call(ctx, "account/rateLimits/read", map[string]any{
		"supportsLunaReserve":       false,
		"excludeResetCreditDetails": true,
	}, &result); err != nil {
		return err
	}
	if len(result.RateLimits) == 0 || string(result.RateLimits) == "null" {
		a.client.stop(ErrProtocol)
		return ErrProtocol
	}
	return nil
}

func (a *Adapter) StartThread(ctx context.Context, model string) (string, error) {
	return a.startThread(ctx, model, "read-only")
}

func (a *Adapter) StartEngineeringThread(ctx context.Context, model string) (string, error) {
	return a.startThread(ctx, model, "workspace-write")
}

func (a *Adapter) startThread(ctx context.Context, model, sandboxMode string) (string, error) {
	if err := a.enter(ctx); err != nil {
		return "", err
	}
	defer a.leave()
	a.mu.Lock()
	ready, thread := a.initialized, a.thread
	a.mu.Unlock()
	if !ready || thread != "" || strings.TrimSpace(model) == "" || len(model) > 128 ||
		(sandboxMode != "read-only" && sandboxMode != "workspace-write") {
		return "", ErrLifecycle
	}
	work, err := canonicalPath(a.worktree, true)
	if err != nil {
		return "", err
	}
	var result struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = a.client.Call(ctx, "thread/start", map[string]any{
		"cwd": work, "model": model, "approvalPolicy": "never",
		"sandbox": sandboxMode, "ephemeral": true,
	}, &result); err != nil {
		return "", err
	}
	if !remoteID(result.Thread.ID) {
		a.client.stop(ErrProtocol)
		return "", ErrProtocol
	}
	a.mu.Lock()
	a.thread = result.Thread.ID
	a.mu.Unlock()
	return result.Thread.ID, nil
}

func textInput(text string) ([]map[string]string, error) {
	if strings.TrimSpace(text) == "" || !utf8.ValidString(text) || len(text) > 64<<10 {
		return nil, fmt.Errorf("bounded nonempty UTF-8 input required")
	}
	return []map[string]string{{"type": "text", "text": text}}, nil
}
func remoteID(id string) bool { return id != "" && len(id) <= 256 && strings.TrimSpace(id) == id }
func (a *Adapter) StartTurn(ctx context.Context, text string) (string, error) {
	input, err := textInput(text)
	if err != nil {
		return "", err
	}
	if err = a.enter(ctx); err != nil {
		return "", err
	}
	defer a.leave()
	a.mu.Lock()
	thread, turn := a.thread, a.turn
	a.mu.Unlock()
	if thread == "" || turn != "" {
		return "", ErrLifecycle
	}
	a.mu.Lock()
	a.starting = true
	a.earlyCompletion = ""
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.starting = false; a.mu.Unlock() }()
	var result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = a.client.Call(ctx, "turn/start", map[string]any{"threadId": thread, "input": input}, &result); err != nil {
		return "", err
	}
	if !remoteID(result.Turn.ID) {
		a.client.stop(ErrProtocol)
		return "", ErrProtocol
	}
	a.mu.Lock()
	if a.earlyCompletion != "" && a.earlyCompletion != result.Turn.ID {
		a.mu.Unlock()
		a.client.stop(ErrProtocol)
		return "", ErrProtocol
	}
	a.turn = result.Turn.ID
	if a.earlyCompletion == result.Turn.ID {
		a.turn = ""
	}
	a.earlyCompletion = ""
	a.mu.Unlock()
	return result.Turn.ID, nil
}
func (a *Adapter) Steer(ctx context.Context, expectedTurn, text string) error {
	input, err := textInput(text)
	if err != nil {
		return err
	}
	if err = a.enter(ctx); err != nil {
		return err
	}
	defer a.leave()
	a.mu.Lock()
	thread, turn := a.thread, a.turn
	a.mu.Unlock()
	if expectedTurn == "" || expectedTurn != turn {
		return ErrLifecycle
	}
	var result struct {
		TurnID string `json:"turnId"`
	}
	if err = a.client.Call(ctx, "turn/steer", map[string]any{"threadId": thread, "expectedTurnId": turn, "input": input}, &result); err != nil {
		return err
	}
	if result.TurnID != turn {
		a.client.stop(ErrProtocol)
		return ErrProtocol
	}
	return nil
}
func (a *Adapter) Interrupt(ctx context.Context, expectedTurn string) error {
	if err := a.enter(ctx); err != nil {
		return err
	}
	defer a.leave()
	a.mu.Lock()
	thread, turn := a.thread, a.turn
	a.mu.Unlock()
	if expectedTurn == "" || expectedTurn != turn {
		return ErrLifecycle
	}
	// An ACK does not mean completion. Only matching turn/completed ends a turn.
	return a.client.Call(ctx, "turn/interrupt", map[string]string{"threadId": thread, "turnId": turn}, nil)
}

// Next processes provider events. Caller must drain it during calls (approvals
// may block an RPC). Unknown server requests receive method-not-found, never an
// implicit grant. The host retains all notification content as untrusted data.
func (a *Adapter) Next(ctx context.Context) (Event, error) {
	if a == nil || a.client == nil {
		return Event{}, ErrLifecycle
	}
	select {
	case <-ctx.Done():
		return Event{}, ctx.Err()
	case e, ok := <-a.client.Events():
		if !ok {
			return Event{}, ErrClientClosed
		}
		if e.Kind == ServerRequest {
			var scope struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
			}
			err := json.Unmarshal(e.Message.Params, &scope)
			a.mu.Lock()
			thread, turn := a.thread, a.turn
			a.mu.Unlock()
			known := e.Message.Method == "item/commandExecution/requestApproval" || e.Message.Method == "item/fileChange/requestApproval"
			if known && err == nil && scope.ThreadID == thread && scope.TurnID == turn && turn != "" {
				err = a.client.Respond(ctx, e.Message.ID, map[string]string{"decision": "decline"}, nil)
			} else {
				err = a.client.Respond(ctx, e.Message.ID, nil, &RPCError{Code: -32601, Message: "request is not authorized by this read-only adapter"})
			}
			return e, err
		}
		if e.Kind == Notification && e.Message.Method == "turn/completed" {
			var p struct {
				ThreadID string `json:"threadId"`
				Turn     struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"turn"`
			}
			if json.Unmarshal(e.Message.Params, &p) != nil {
				return e, ErrProtocol
			}
			a.mu.Lock()
			defer a.mu.Unlock()
			if p.ThreadID != a.thread || !remoteID(p.Turn.ID) {
				return e, ErrLifecycle
			}
			if p.Turn.Status != "completed" && p.Turn.Status != "interrupted" && p.Turn.Status != "failed" {
				return e, ErrProtocol
			}
			// A provider may finish before the start response reaches this goroutine.
			if a.starting && a.turn == "" {
				if a.earlyCompletion != "" {
					return e, ErrLifecycle
				}
				a.earlyCompletion = p.Turn.ID
			} else {
				if p.Turn.ID != a.turn || a.turn == "" {
					return e, ErrLifecycle
				}
				a.turn = ""
			}
		}
		return e, nil
	}
}
