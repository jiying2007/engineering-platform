package codexapp

import (
	"context"
	"errors"
	"time"

	"github.com/jiying2007/engineering-platform/internal/processscope"
)

var (
	ErrEngineeringStopUnconfirmed = errors.New("runtime process-tree termination unconfirmed")
	ErrEngineeringCloseUnobserved = errors.New("runtime control close acknowledgement unobserved; exact Core readback required")
)

// Both credential lanes settle the same original process/control lifetime here.
// Calls are synchronous, one-attempt and independently bounded; no execution,
// publication, scope creation or retry is authorized by this cleanup. A synthetic
// proof supplied in unit tests is not promoted to production quiescence evidence.
func settleEngineeringExit(ctx context.Context, stopScope func(context.Context) (processscope.Proof, error), proof processscope.Proof, controller EngineeringController, started bool, status string, prior error) (processscope.Proof, error) {
	if !proof.Quiescent() {
		stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		var err error
		proof, err = stopScope(stopping)
		cancel()
		if err != nil || !proof.Quiescent() {
			prior = errors.Join(prior, ErrEngineeringStopUnconfirmed, err)
		}
	}
	if controller != nil && started {
		if status == "" {
			status = "unknown"
		}
		reporting, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
		err := controller.Close(reporting, status, proof)
		cancel()
		if err != nil {
			prior = errors.Join(prior, ErrEngineeringCloseUnobserved, err)
		}
	}
	return proof, prior
}
