//go:build !linux

package main

import (
	"context"
	"fmt"
)

func openProductionObserverJournal(ctx context.Context, _ string) (productionObserverJournalInput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("observer journal inspection requires the qualified Linux file-read boundary")
}
