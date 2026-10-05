package codexapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// These tests exercise real correlated JSONL replies and terminal events over
// net.Pipe. The controller is an explicit fixture, not Core/provider evidence.
// A terminal event is not an ACK and must not cancel a sent call's bounded read.
func TestEngineeringTerminalAndControlReplyOrdering(t *testing.T) {
	for _, kind := range []string{"INTERRUPT", "STEER"} {
		for _, order := range []string{"reply-first", "terminal-first"} {
			t.Run(kind+"/"+order, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				c, remote := pair(t)
				defer c.Close()
				defer remote.Close()
				a, err := NewAdapter(c, t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				a.initialized, a.thread, a.turn = true, "thread", "turn"
				control := &fixtureController{bound: make(chan struct{}), reported: make(chan struct{}), command: &LiveControl{ID: "one", Kind: kind, ThreadID: "thread", TurnID: "turn", Text: "test-only correction"}}
				type observed struct {
					value EngineeringObservation
					err   error
				}
				done := make(chan observed, 1)
				go func() {
					value, err := observeControlledEngineering(ctx, a, "thread", "turn", control)
					done <- observed{value, err}
				}()
				defer func() { cancel(); c.Close() }()
				if err := remote.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
					t.Fatal(err)
				}
				var request Message
				if err := json.NewDecoder(remote).Decode(&request); err != nil {
					t.Fatal(err)
				}
				method := "turn/" + strings.ToLower(kind)
				if request.Method != method || !validID(request.ID) {
					t.Fatal("wrong control request")
				}
				result := `{}`
				status := "interrupted"
				terminal := ""
				if kind == "STEER" {
					result = `{"turnId":"turn"}`
					status = "completed"
					terminal = `{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"result","type":"agentMessage","text":"test-only result"}}}` + "\n"
				}
				terminal += `{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"` + status + `"}}}` + "\n"
				ack := `{"id":` + string(request.ID) + `,"result":` + result + `}` + "\n"
				if order == "reply-first" {
					_, err = io.WriteString(remote, ack+terminal)
				} else {
					_, err = io.WriteString(remote, terminal)
					if err != nil {
						t.Fatal(err)
					}
					// This deliberate withheld response proves that terminal observation is
					// NOT a control receipt. It does not increase the production RPC budget.
					select {
					case <-control.reported:
						t.Fatal("terminal event prematurely settled the in-flight control")
					case <-time.After(25 * time.Millisecond):
					}
					_, err = io.WriteString(remote, ack)
				}
				if err != nil {
					t.Fatal(err)
				}
				select {
				case got := <-done:
					if got.value.Status != status {
						t.Fatalf("terminal event lost: %+v %v", got.value, got.err)
					}
					if kind == "INTERRUPT" && !errors.Is(got.err, ErrEngineeringInterrupted) {
						t.Fatalf("interruption outcome lost: %v", got.err)
					}
					if kind == "STEER" && got.err != nil {
						t.Fatalf("valid steering receipt lost: %v", got.err)
					}
				case <-ctx.Done():
					t.Fatal("control and terminal observation did not join")
				}
				control.mu.Lock()
				defer control.mu.Unlock()
				want := "INTERRUPT_ACKNOWLEDGED"
				if kind == "STEER" {
					want = "STEER_ACCEPTED"
				}
				if len(control.outcomes) != 1 || control.outcomes[0] != want || control.claimed != 1 {
					t.Fatalf("control response overwritten/replayed: %v", control.outcomes)
				}
			})
		}
	}
}

func TestEngineeringTerminalDoesNotInventMissingControlReply(t *testing.T) {
	for _, mode := range []string{"lost-reply", "parent-cancel", "rpc-error", "foreign-terminal"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			c, remote := pair(t)
			defer c.Close()
			defer remote.Close()
			a, err := NewAdapter(c, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			a.initialized, a.thread, a.turn = true, "thread", "turn"
			control := &fixtureController{bound: make(chan struct{}), reported: make(chan struct{}), command: &LiveControl{ID: "one", Kind: "INTERRUPT", ThreadID: "thread", TurnID: "turn"}}
			done := make(chan error, 1)
			go func() { _, e := observeControlledEngineering(ctx, a, "thread", "turn", control); done <- e }()
			if err := remote.SetDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			var request Message
			if err = json.NewDecoder(remote).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Method != "turn/interrupt" {
				t.Fatal("unexpected control method")
			}
			terminal := `{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"interrupted"}}}` + "\n"
			switch mode {
			case "parent-cancel":
				cancel()
			case "foreign-terminal":
				_, err = io.WriteString(remote, strings.Replace(terminal, `"threadId":"thread"`, `"threadId":"other"`, 1))
			case "lost-reply":
				_, err = io.WriteString(remote, terminal)
				remote.Close()
			case "rpc-error":
				_, err = io.WriteString(remote, terminal+`{"id":`+string(request.ID)+`,"error":{"code":-32000,"message":"test-only failure"}}`+"\n")
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("unconfirmed/invalid control returned success")
				}
			case <-ctx.Done():
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("cancellation did not join")
				}
			}
			control.mu.Lock()
			defer control.mu.Unlock()
			if len(control.outcomes) != 1 || control.outcomes[0] != "UNKNOWN" || control.claimed != 1 {
				t.Fatalf("lost response promoted or replayed: %v", control.outcomes)
			}
		})
	}
}
