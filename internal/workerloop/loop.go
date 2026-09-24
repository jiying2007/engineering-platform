// Package workerloop provides bounded cooperative polling. It never detaches a
// callback on cancellation; callbacks must honor their context and finish.
package workerloop

import (
	"context"
	"fmt"
	"time"
)

func Run(ctx context.Context, interval time.Duration, step func(context.Context) error) error {
	if interval < 10*time.Millisecond || interval > time.Minute || step == nil {
		return fmt.Errorf("bounded polling interval and step required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := step(ctx); err != nil {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
