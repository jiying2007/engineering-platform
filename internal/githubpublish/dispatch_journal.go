package githubpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// This directory holds immutable Publisher witness bytes, not an execution
// queue or a second Core ledger. Existing/partial slots always block Publish.
// Never auto-delete or reuse them, including across process restarts.
type dispatchJournal struct {
	path     string
	identity os.FileInfo
}

func newDispatchJournal(path string) (*dispatchJournal, error) {
	p, info, err := canonicalDirectory(path)
	if err != nil || !witnessPrivateDirectory(info) {
		return nil, fmt.Errorf("publisher witness root must be a pre-created canonical private 0700 directory")
	}
	return &dispatchJournal{p, info}, nil
}
func (j *dispatchJournal) open() (*os.Root, error) {
	if j == nil {
		return nil, ErrDispatchEvidenceUnavailable
	}
	current, err := os.Lstat(j.path)
	if err != nil || !witnessPrivateDirectory(current) || !os.SameFile(current, j.identity) {
		return nil, ErrDispatchEvidenceUnavailable
	}
	r, err := os.OpenRoot(j.path)
	if err != nil {
		return nil, err
	}
	opened, err := r.Stat(".")
	if err != nil || !witnessPrivateDirectory(opened) || !os.SameFile(current, opened) {
		r.Close()
		return nil, ErrDispatchEvidenceUnavailable
	}
	return r, nil
}
func dispatchSlot(plan Plan) string {
	digest, _ := canonical.Digest(struct {
		Repository  string
		ExecutionID string
	}{plan.Repository, plan.ExecutionID})
	return strings.TrimPrefix(digest, "sha256:")
}
func syncRoot(r *os.Root) error {
	f, err := r.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
func writeWitness(r *os.Root, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 || len(raw) > 64<<10 {
		return ErrDispatchEvidenceUnavailable
	}
	f, err := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, werr := f.Write(raw)
	if n != len(raw) && werr == nil {
		werr = io.ErrShortWrite
	}
	err = errors.Join(werr, f.Sync(), f.Close())
	if err != nil {
		return err
	} // Keep incomplete bytes and occupied slot, never retry.
	return syncRoot(r)
}
func readWitness(r *os.Root, name string, dst any) error {
	before, err := r.Lstat(name)
	if err != nil || !witnessPrivateFile(before) {
		return ErrDispatchEvidenceUnavailable
	}
	f, err := openWitnessRead(r, name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !witnessPrivateFile(info) || !os.SameFile(before, info) {
		return ErrDispatchEvidenceUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return ErrDispatchEvidenceUnavailable
	}
	after, err := r.Lstat(name)
	if err != nil || !witnessPrivateFile(after) || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || int64(len(raw)) != before.Size() {
		return ErrDispatchEvidenceUnavailable
	}
	if err := strictjson.Decode(raw, dst); err != nil {
		return ErrDispatchEvidenceUnavailable
	}
	// A prior failed fsync is never inferred to have succeeded; exact readback
	// re-establishes durability before Core may retain these observed bytes.
	canonicalBytes, marshalErr := json.Marshal(dst)
	if marshalErr != nil || !bytes.Equal(raw, canonicalBytes) {
		return ErrDispatchEvidenceUnavailable
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return syncRoot(r)
}
func (j *dispatchJournal) begin(ctx context.Context, identity DispatchIdentity, plan Plan) (*dispatchLease, DispatchStart, error) {
	var zero DispatchStart
	if err := ctx.Err(); err != nil {
		return nil, zero, err
	}
	start := DispatchStart{Version: 2, Identity: identity, Plan: plan, StartedAt: time.Now().UTC()}
	if identity.Validate() != nil || plan.Validate() != nil || identity.RunID != plan.RunID {
		return nil, zero, ErrDispatchEvidenceUnavailable
	}
	root, err := j.open()
	if err != nil {
		return nil, zero, err
	}
	defer root.Close()
	name := dispatchSlot(plan)
	if err := root.Mkdir(name, 0700); err != nil {
		return nil, zero, errors.Join(ErrDispatchEvidenceUnavailable, err)
	}
	// From here on every failure leaves an occupied non-reusable slot.
	slot, err := root.OpenRoot(name)
	if err != nil {
		return nil, zero, err
	}
	if err := syncRoot(root); err != nil {
		slot.Close()
		return nil, zero, err
	}
	if err := ctx.Err(); err != nil {
		slot.Close()
		return nil, zero, err
	}
	info, err := slot.Stat(".")
	if err != nil || !witnessPrivateDirectory(info) {
		slot.Close()
		return nil, zero, ErrDispatchEvidenceUnavailable
	}
	lock, err := lockWitnessSlot(slot)
	if err != nil {
		slot.Close()
		return nil, zero, err
	}
	lease := &dispatchLease{Root: slot, lock: lock}
	start.Incarnation, err = witnessIncarnation(slot)
	if err != nil || start.Validate() != nil {
		lease.Close()
		return nil, zero, ErrDispatchEvidenceUnavailable
	}
	if err := writeWitness(slot, "started.json", start); err != nil {
		lease.Close()
		return nil, zero, err
	}
	return lease, start, nil
}
func (j *dispatchJournal) completed(ctx context.Context, identity DispatchIdentity, plan Plan) (DispatchStart, DispatchCompletion, error) {
	var s DispatchStart
	var c DispatchCompletion
	if identity.Validate() != nil || plan.Validate() != nil {
		return s, c, ErrDispatchEvidenceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return s, c, err
	}
	root, err := j.open()
	if err != nil {
		return s, c, err
	}
	defer root.Close()
	name := dispatchSlot(plan)
	before, err := root.Lstat(name)
	if err != nil || !witnessPrivateDirectory(before) {
		return s, c, ErrDispatchEvidenceUnavailable
	}
	slot, err := root.OpenRoot(name)
	if err != nil {
		return s, c, err
	}
	defer slot.Close()
	opened, err := slot.Stat(".")
	if err != nil || !witnessPrivateDirectory(opened) || !os.SameFile(before, opened) {
		return s, c, ErrDispatchEvidenceUnavailable
	}
	if err := readWitness(slot, "started.json", &s); err != nil {
		return s, c, err
	}
	if s.Validate() != nil || s.Identity != identity || s.Plan != plan {
		return s, c, ErrDispatchEvidenceUnavailable
	}
	if err := readWitness(slot, "completed.json", &c); err != nil {
		return s, c, err
	}
	if c.Validate(s) != nil {
		return s, c, ErrDispatchEvidenceUnavailable
	}
	if s.Version == 2 {
		var invoked DispatchInvocation
		if readWitness(slot, "invoked.json", &invoked) != nil || invoked.Validate(s) != nil ||
			invoked.ArmedAt.After(c.ReturnedAt) || !witnessAbsent(slot, "not-invoked.json") {
			return s, c, ErrDispatchEvidenceUnavailable
		}
	}
	if err := syncRoot(root); err != nil {
		return s, c, err
	}
	if err := ctx.Err(); err != nil {
		return s, c, err
	}
	return s, c, nil
}
