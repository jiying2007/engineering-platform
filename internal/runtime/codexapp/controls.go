package codexapp

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrEngineeringInterrupted = errors.New("engineering turn interrupted; no deliverable result")

// These are transport observations, not Task/Run/epoch authority. The Worker
// bridge must persist a claim before returning it and must never redeliver it.
type LiveControl struct{ ID, Kind, Text, ThreadID, TurnID string }
type EngineeringController interface {
	Bind(context.Context, string, string) error
	Claim(context.Context) (*LiveControl, error)
	Report(context.Context, string, string) error
	Close(context.Context, string, bool) error
}

func observeControlledEngineering(ctx context.Context, a *Adapter, threadID, turnID string, c EngineeringController) (EngineeringObservation, error) {
	if c == nil {
		return ObserveEngineeringTurn(ctx, a, threadID, turnID)
	}
	if err := c.Bind(ctx, threadID, turnID); err != nil {
		return EngineeringObservation{}, err
	}
	running, cancel := context.WithCancel(ctx)
	defer cancel()
	type observed struct {
		value EngineeringObservation
		err   error
	}
	observations := make(chan observed, 1)
	deliveries := make(chan error, 1)
	go func() { v, e := ObserveEngineeringTurn(running, a, threadID, turnID); observations <- observed{v, e} }()
	go func() { deliveries <- pumpControls(running, a, threadID, turnID, c) }()
	select {
	case result := <-observations:
		cancel()
		dispatchErr := <-deliveries
		if result.err == nil && dispatchErr != nil {
			result.err = dispatchErr
		}
		return result.value, result.err
	case err := <-deliveries:
		cancel()
		result := <-observations
		if err != nil {
			return result.value, err
		}
		if result.err == nil && ctx.Err() != nil {
			result.err = ctx.Err()
		}
		return result.value, result.err
	}
}
func pumpControls(ctx context.Context, a *Adapter, threadID, turnID string, c EngineeringController) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		polling, stop := context.WithTimeout(ctx, 5*time.Second)
		command, err := c.Claim(polling)
		stop()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if command == nil {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		outcome := "UNKNOWN"
		var callErr error
		if command.ID == "" || command.ThreadID != threadID || command.TurnID != turnID {
			return fmt.Errorf("claimed control does not belong to active turn")
		}
		if ctx.Err() != nil {
			outcome = "NOT_APPLIED"
			callErr = ctx.Err()
		} else {
			call, done := context.WithTimeout(ctx, 5*time.Second)
			switch command.Kind {
			case "STEER":
				callErr = a.Steer(call, turnID, command.Text)
				if callErr == nil {
					outcome = "STEER_ACCEPTED"
				}
			case "INTERRUPT":
				callErr = a.Interrupt(call, turnID)
				if callErr == nil {
					outcome = "INTERRUPT_ACKNOWLEDGED"
				}
			default:
				callErr = fmt.Errorf("unsupported live control")
			}
			done()
			// ErrLifecycle is generated before any provider call. Transport/protocol
			// errors, including lost replies, remain UNKNOWN and are never replayed.
			if errors.Is(callErr, ErrLifecycle) {
				outcome = "NOT_APPLIED"
			}
		}
		reporting, done := context.WithTimeout(context.Background(), 5*time.Second)
		err = c.Report(reporting, command.ID, outcome)
		done()
		if err != nil {
			return err
		}
		if callErr != nil {
			return callErr
		}
		if command.Kind == "INTERRUPT" {
			// An empty interrupt response is only an ACK. The event reader must observe
			// matching turn/completed; no second claim or implicit resume is allowed.
			<-ctx.Done()
			return nil
		}
	}
}
