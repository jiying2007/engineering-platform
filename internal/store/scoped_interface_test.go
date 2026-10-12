package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"reflect"
	"testing"
	"time"
)

func TestStoreAuthorityInterfaceHasOnlyCallerContextMethods(t *testing.T) {
	contract := reflect.TypeOf((*Store)(nil)).Elem()
	ctx := reflect.TypeOf((*context.Context)(nil)).Elem()
	for i := 0; i < contract.NumMethod(); i++ {
		method := contract.Method(i)
		if method.Type.NumIn() == 0 || method.Type.In(0) != ctx {
			t.Errorf("authority method %s can discard caller lifecycle", method.Name)
		}
	}
}

func TestAdditionalHistoricalContextsRejectCancelledLockWait(t *testing.T) {
	calls := []func(context.Context, *Memory) error{
		func(ctx context.Context, m *Memory) error {
			_, err := m.GetTaskRevisionContext(ctx, "missing", 1)
			return err
		},
		func(ctx context.Context, m *Memory) error {
			_, err := m.GetAttemptContext(ctx, "missing", "missing")
			return err
		},
		func(ctx context.Context, m *Memory) error {
			return m.RecordSteeringContext(ctx, "missing", 1, run.Run{}, session.Session{}, session.SteeringCommand{})
		},
	}
	for i, call := range calls {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			m := NewMemory()
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &memoryCheckedContext{Context: parent, firstChecked: make(chan struct{})}
			m.mu.Lock()
			held := true
			defer func() {
				if held {
					m.mu.Unlock()
				}
			}()
			done := make(chan error, 1)
			go func() { done <- call(ctx, m) }()
			select {
			case <-ctx.firstChecked:
			case <-time.After(2 * time.Second):
				t.Fatal("missing initial check")
			}
			cancel()
			m.mu.Unlock()
			held = false
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancelled call accessed ledger", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("call did not exit")
			}
		})
	}
}

func TestHistoricalSteeringCannotSubstituteRunOrSession(t *testing.T) {
	for _, bad := range []string{"run", "session"} {
		t.Run(bad, func(t *testing.T) {
			m := NewMemory()
			value := run.Run{ID: "run", Version: 1, CurrentEpoch: 1}
			sess := session.Session{RunID: "run", ExecutionEpoch: 1}
			m.runs[value.ID] = value
			m.sessions[value.ID] = sess
			sess.LastSequence = 1
			if bad == "run" {
				value.ID = "other"
			} else {
				sess.RunID = "other"
			}
			cmd := session.SteeringCommand{ID: "steer", RunID: "run", ExecutionEpoch: 1, Sequence: 1}
			if err := m.RecordSteeringContext(context.Background(), "run", 1, value, sess, cmd); !errors.Is(err, ErrConflict) {
				t.Fatal("cross-run identity accepted", err)
			}
			if m.runs["run"].Version != 1 || m.sessions["run"].LastSequence != 0 || len(m.steeringCommands) != 0 {
				t.Fatal("failed identity check changed ledger")
			}
		})
	}
}
