package codexapp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type fixtureController struct {
	mu            sync.Mutex
	command       *LiveControl
	claimed       int
	outcomes      []string
	bound         chan struct{}
	reported      chan struct{}
	closed        string
	exited        bool
	reportFailure bool
	closeFailure  bool
}

func (c *fixtureController) Bind(_ context.Context, thread, turn string) error {
	if thread != "thread" || turn != "turn" {
		return errors.New("fixture binding mismatch")
	}
	close(c.bound)
	return nil
}
func (c *fixtureController) Claim(context.Context) (*LiveControl, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claimed > 0 {
		return nil, nil
	}
	c.claimed++
	return c.command, nil
}
func (c *fixtureController) Report(_ context.Context, id, outcome string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outcomes = append(c.outcomes, outcome)
	close(c.reported)
	if c.reportFailure {
		return errors.New("fixture lost Core report response")
	}
	return nil
}
func (c *fixtureController) Close(_ context.Context, status string, exited bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = status
	c.exited = exited
	if c.closeFailure {
		return errors.New("fixture Core seal unavailable")
	}
	return nil
}

func TestEngineeringActualProcessControls(t *testing.T) {
	for _, mode := range []string{"steer", "interrupt", "lost-reply", "report-failure", "close-failure", "foreign-turn"} {
		t.Run(mode, func(t *testing.T) {
			python, err := exec.LookPath("python3")
			if err != nil {
				t.Fatal(err)
			}
			base := t.TempDir()
			work := filepath.Join(base, "work")
			home := filepath.Join(base, "home")
			auth := filepath.Join(base, "auth")
			for _, dir := range []string{work, home, auth} {
				if err = os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			executable := filepath.Join(base, "fixture-codex")
			raw := []byte("#!" + python + " -S\n" + testsupport.ControlledCodexProtocol)
			if err = os.WriteFile(executable, raw, 0700); err != nil {
				t.Fatal(err)
			}
			login := filepath.Join(auth, "auth.json")
			if err = os.WriteFile(login, []byte(`{"fixture_only":"never-an-account"}`), 0600); err != nil {
				t.Fatal(err)
			}
			controller := &fixtureController{bound: make(chan struct{}), reported: make(chan struct{}), command: &LiveControl{ID: "c1", Kind: "STEER", Text: "只修复故障", ThreadID: "thread", TurnID: "turn"}, reportFailure: mode == "report-failure", closeFailure: mode == "close-failure"}
			if mode == "interrupt" {
				controller.command.Kind = "INTERRUPT"
				controller.command.Text = ""
			}
			if mode == "foreign-turn" {
				controller.command.TurnID = "other"
			}
			if mode == "lost-reply" {
				if err = os.WriteFile(filepath.Join(work, "drop-reply"), []byte("test"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			type outcome struct {
				receipt EngineeringReceipt
				err     error
			}
			done := make(chan outcome, 1)
			go func() {
				r, e := EngineeringSavedLoginTurn(ctx, executable, "0.157.1", canonical.BytesDigest(raw), canonical.BytesDigest([]byte("fixture-only-qualification")), work, home, login, "fixture-only-model", "fixture prompt", controller)
				done <- outcome{r, e}
			}()
			select {
			case <-controller.bound:
			case <-ctx.Done():
				t.Fatal("fixture not bound")
			}
			if mode == "lost-reply" {
				until := time.Now().Add(2 * time.Second)
				for {
					if _, err = os.Stat(filepath.Join(work, "requests.jsonl")); err == nil {
						break
					}
					if time.Now().After(until) {
						t.Fatal("request never dispatched")
					}
					time.Sleep(5 * time.Millisecond)
				}
				cancel()
			} else if mode != "foreign-turn" && mode != "report-failure" {
				select {
				case <-controller.reported:
				case <-ctx.Done():
					t.Fatal("no provider receipt")
				}
				controller.mu.Lock()
				closed := controller.closed
				controller.mu.Unlock()
				if closed != "" {
					t.Fatal("control ACK was treated as terminal event")
				}
				if err = os.WriteFile(filepath.Join(work, "allow-finish"), []byte("test barrier"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var result outcome
			select {
			case result = <-done:
			case <-time.After(8 * time.Second):
				t.Fatal("process join did not complete")
			}
			if mode == "steer" {
				if result.err != nil || result.receipt.TurnStatus != "completed" {
					t.Fatalf("%+v", result)
				}
			} else if result.err == nil {
				t.Fatal("non-deliverable outcome returned success")
			}
			controller.mu.Lock()
			defer controller.mu.Unlock()
			if !controller.exited {
				t.Fatal("missing app-server process exit observation")
			}
			if mode == "interrupt" && (controller.closed != "interrupted" || len(controller.outcomes) != 1 || controller.outcomes[0] != "INTERRUPT_ACKNOWLEDGED") {
				t.Fatal("interruption event lost", controller.closed, controller.outcomes)
			}
			if mode == "lost-reply" && (len(controller.outcomes) != 1 || controller.outcomes[0] != "UNKNOWN") {
				t.Fatal("lost RPC reply did not remain UNKNOWN")
			}
			if mode == "foreign-turn" {
				if _, err = os.Stat(filepath.Join(work, "requests.jsonl")); !os.IsNotExist(err) {
					t.Fatal("foreign turn reached provider")
				}
				return
			}
			requests, err := os.ReadFile(filepath.Join(work, "requests.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if len(strings.Split(strings.TrimSpace(string(requests)), "\n")) != 1 || controller.claimed != 1 {
				t.Fatal("control RPC replayed")
			}
		})
	}
}
