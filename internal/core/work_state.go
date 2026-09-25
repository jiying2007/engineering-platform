package core

import "errors"

var ErrInvalidWorkTransition = errors.New("invalid work state transition")

func (w *WorkItem) Transition(to WorkState) error {
	if w.State == to {
		return nil
	}
	allowed := false
	switch w.State {
	case WorkDraft:
		allowed = to == WorkReady || to == WorkCancelled
	case WorkReady:
		allowed = to == WorkExecuting || to == WorkCancelled
	case WorkExecuting:
		allowed = to == WorkVerifying || to == WorkCancelled
	case WorkVerifying:
		allowed = to == WorkReviewing || to == WorkCancelled
	case WorkReviewing:
		allowed = to == WorkClosed || to == WorkCancelled
	}
	if !allowed {
		return ErrInvalidWorkTransition
	}
	w.State = to
	return nil
}
