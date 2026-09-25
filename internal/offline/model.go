// Package offline owns a single, non-replayable, offline execution reservation.
// A preparation receipt never grants execution; a fresh live Core check does.
package offline

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

const Action = "worker.offline-execute"
const Kind = "WORKER_ATTESTED_OFFLINE_EXECUTION"
const Authorized = "AUTHORIZED"
const Finished = "FINISHED"
const Unknown = "UNKNOWN"

type Start struct {
	RunID         string          `json:"run_id"`
	WorkerProfile string          `json:"worker_profile"`
	Profile       sandbox.Profile `json:"profile"`
}
type Token struct {
	ID            string `json:"execution_id"`
	RunID         string `json:"run_id"`
	WorkerProfile string `json:"worker_profile"`
	ProfileDigest string `json:"profile_digest"`
	RecoveryEpoch uint64 `json:"recovery_epoch"`
}

func (t Token) Valid() bool {
	return regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(t.ID) && t.RunID != "" && len(t.RunID) <= 256 && workerqueue.ValidProfile(t.WorkerProfile) && canonical.ValidDigest(t.ProfileDigest)
}

type Permit struct {
	Token       Token                  `json:"token"`
	Assignment  workerqueue.Assignment `json:"assignment"`
	Preparation preparation.Receipt    `json:"preparation"`
	Profile     sandbox.Profile        `json:"profile"`
	LeaseUntil  time.Time              `json:"lease_until"`
}

func (p Permit) Check(subject string, request Start) error {
	digest, err := p.Profile.Digest()
	want, werr := request.Profile.Digest()
	if err != nil || werr != nil || digest != want || !p.Token.Valid() || p.Token.ProfileDigest != digest || p.Token.WorkerProfile != request.WorkerProfile || p.Token.RunID != request.RunID || p.Assignment.Intent.RunID != request.RunID || p.Assignment.Input.WorkerProfile != request.WorkerProfile || p.LeaseUntil.IsZero() {
		return workerqueue.ErrIdentity
	}
	if err := CheckAssignment(p.Assignment, p.Profile); err != nil {
		return err
	}
	return preparation.Verify(p.Assignment, p.Preparation, p.Preparation.Facts, subject)
}

type Report struct {
	Token  Token          `json:"token"`
	Result sandbox.Result `json:"result"`
}
type Receipt struct {
	Kind              string         `json:"kind"`
	Token             Token          `json:"token"`
	Worker            string         `json:"worker_subject"`
	PreparationDigest string         `json:"preparation_digest"`
	Result            sandbox.Result `json:"result"`
	ResultDigest      string         `json:"result_digest"`
	ReceivedAt        time.Time      `json:"received_at"`
}

func (r Receipt) Verify(subject string, p Permit, result sandbox.Result) error {
	digest, err := canonical.Digest(result)
	actual, aerr := canonical.Digest(r.Result)
	if err != nil || aerr != nil || digest != actual || r.ResultDigest != digest || r.Kind != Kind || r.Worker != subject || r.Token != p.Token || r.PreparationDigest != p.Preparation.FactsDigest || r.ReceivedAt.IsZero() || r.Result.Validate(p.Profile) != nil {
		return fmt.Errorf("%w: offline receipt mismatch", workerqueue.ErrIdentity)
	}
	return nil
}

type Status struct {
	Token      Token     `json:"token"`
	State      string    `json:"state"`
	LeaseUntil time.Time `json:"lease_until"`
	Receipt    *Receipt  `json:"receipt,omitempty"`
}
type Repository interface {
	StartOffline(context.Context, string, Start) (Permit, error)
	RenewOffline(context.Context, string, Token) (time.Time, error)
	FinishOffline(context.Context, string, Report) (Receipt, error)
	FailOffline(context.Context, string, Token) error
	GetOffline(context.Context, string) (Status, error)
}

// The immutable task and Run input must opt into the same exact offline profile.
func CheckAssignment(a workerqueue.Assignment, p sandbox.Profile) error {
	d, err := p.Digest()
	if err != nil {
		return err
	}
	if _, err = workerqueue.Validate(a); err != nil {
		return err
	}
	if a.Input.ToolProfile != "offline/"+d {
		return workerqueue.ErrIdentity
	}
	for _, allowed := range a.Task.AllowedActions {
		if allowed == Action {
			return nil
		}
	}
	return workerqueue.ErrIdentity
}
