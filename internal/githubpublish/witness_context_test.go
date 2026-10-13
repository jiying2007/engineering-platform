//go:build linux

package githubpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type cancellingWitnessValue struct {
	Value  string
	cancel context.CancelFunc
}

func (v cancellingWitnessValue) MarshalJSON() ([]byte, error) {
	if v.cancel != nil {
		v.cancel()
	}
	return json.Marshal(struct {
		Value string `json:"value"`
	}{v.Value})
}

func (v *cancellingWitnessValue) UnmarshalJSON(raw []byte) error {
	var value struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	v.Value = value.Value
	if v.cancel != nil {
		v.cancel()
	}
	return nil
}

func witnessContextRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, dir
}

func TestPublisherWitnessIntegrityContextPreCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// nil root makes any accidental filesystem access fail immediately.
	if err := writeWitness(ctx, nil, "not-created.json", struct{}{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled writer accessed root", err)
	}
	var value map[string]string
	if err := readWitness(ctx, nil, "not-read.json", &value); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled reader accessed root", err)
	}
}

func TestPublisherWitnessIntegrityContextWriteMarshallingCancellation(t *testing.T) {
	root, dir := witnessContextRoot(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := writeWitness(ctx, root, "not-created.json", cancellingWitnessValue{"ok", cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("marshal cancellation lost", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled marshal reserved immutable name", entries, err)
	}
}

func TestPublisherWitnessIntegrityContextReadDecodeCancellation(t *testing.T) {
	root, dir := witnessContextRoot(t)
	original := []byte(`{"value":"ok"}`)
	if err := os.WriteFile(filepath.Join(dir, "witness.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	value := cancellingWitnessValue{cancel: cancel}
	if err := readWitness(ctx, root, "witness.json", &value); !errors.Is(err, context.Canceled) {
		t.Fatal("decoded cancelled witness accepted", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "witness.json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("cancelled observation changed bytes", err)
	}
	var valid map[string]string
	if err := readWitness(context.Background(), root, "witness.json", &valid); err != nil || valid["value"] != "ok" {
		t.Fatal("cancelled read destroyed later independent readback", valid, err)
	}
}

type witnessCheckpointContext struct {
	context.Context
	cancel context.CancelFunc
	at     int
	checks int
}

func (c *witnessCheckpointContext) Err() error {
	c.checks++
	if c.checks == c.at {
		c.cancel()
	}
	return c.Context.Err()
}

func TestPublisherWitnessIntegrityContextReadCheckpoints(t *testing.T) {
	root, dir := witnessContextRoot(t)
	if err := os.WriteFile(filepath.Join(dir, "witness.json"), []byte(`{"value":"ok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Obtain the actual bounded-read checkpoint count, then cancel separately
	// at every one. No wall-clock race or arbitrary scheduling sleep is used.
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe := &witnessCheckpointContext{Context: parent, cancel: cancel, at: -1}
	var value map[string]string
	if err := readWitness(probe, root, "witness.json", &value); err != nil {
		t.Fatal(err)
	}
	if probe.checks < 6 || probe.checks > 32 {
		t.Fatalf("unexpected evidence checkpoint coverage: %d", probe.checks)
	}
	for at := 1; at <= probe.checks; at++ {
		t.Run(fmt.Sprintf("checkpoint-%d", at), func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &witnessCheckpointContext{Context: parent, cancel: cancel, at: at}
			var value map[string]string
			if err := readWitness(ctx, root, "witness.json", &value); !errors.Is(err, context.Canceled) {
				t.Fatalf("checkpoint %d returned usable readback: %v", at, err)
			}
		})
	}
}

func TestPublisherWitnessIntegrityContextReadNoDescriptorLeak(t *testing.T) {
	root, dir := witnessContextRoot(t)
	if err := os.WriteFile(filepath.Join(dir, "witness.json"), []byte(`{"value":"ok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		parent, cancel := context.WithCancel(context.Background())
		ctx := &witnessCheckpointContext{Context: parent, cancel: cancel, at: 3}
		var value map[string]string
		err := readWitness(ctx, root, "witness.json", &value)
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatal("expected open-descriptor cancellation", err)
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(after) > len(before) {
		t.Fatalf("descriptor leak: before=%d after=%d err=%v", len(before), len(after), err)
	}
}

type cancelAfterOriginalPublish struct {
	*serviceRemote
	cancel context.CancelFunc
}

func (r *cancelAfterOriginalPublish) Publish(ctx context.Context, p Plan, bundle string, id DispatchIdentity) (PublicationReceipt, error) {
	receipt, err := r.serviceRemote.Publish(ctx, p, bundle, id)
	r.cancel()
	return receipt, err
}

func TestPublisherWitnessIntegrityContextOriginalCompletionSettlement(t *testing.T) {
	s, p := publisherServiceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remote := &cancelAfterOriginalPublish{s.remote.(*serviceRemote), cancel}
	s.remote = remote
	raw, err := json.Marshal(publishRequest{Plan: p, Identity: testDispatchIdentity(p)})
	if err != nil {
		t.Fatal(err)
	}
	req := authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", raw).WithContext(ctx)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK || !errors.Is(ctx.Err(), context.Canceled) || remote.publishCalls != 1 {
		t.Fatalf("original completed call lost settlement: code=%d calls=%d err=%v body=%s", response.Code, remote.publishCalls, ctx.Err(), response.Body.String())
	}
	start, completed, err := s.journal.completed(context.Background(), testDispatchIdentity(p), p)
	if err != nil || completed.ValidateForRecovery(start) != nil {
		t.Fatal("independent settlement did not retain original evidence", err)
	}
	before, err := os.ReadFile(filepath.Join(s.journal.path, dispatchSlot(p), "completed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.journal.completed(ctx, testDispatchIdentity(p), p); !errors.Is(err, context.Canceled) {
		t.Fatal("later cancelled readback was accepted", err)
	}
	after, err := os.ReadFile(filepath.Join(s.journal.path, dispatchSlot(p), "completed.json"))
	if err != nil || !bytes.Equal(before, after) || remote.publishCalls != 1 {
		t.Fatal("readback changed/replayed original completion", err)
	}
}
