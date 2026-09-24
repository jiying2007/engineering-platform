package workeragent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

type fakeTransport struct {
	a                         workerqueue.Assignment
	claims, renewals, reports int
	firstReportErr, renewErr  error
	foreign                   bool
	lastReport                workerqueue.Report
}

func (f *fakeTransport) Subject() string { return "urn:engineering-platform:worker:test" }
func (f *fakeTransport) Call(ctx context.Context, method, p string, in, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var result any
	switch p {
	case "/api/v1/worker/claim":
		f.claims++
		result = struct {
			Assignment *workerqueue.Assignment `json:"assignment"`
		}{&f.a}
	case "/api/v1/worker/renew":
		f.renewals++
		if f.renewErr != nil {
			return f.renewErr
		}
		result = struct {
			LeaseUntil time.Time `json:"lease_until"`
		}{time.Now().Add(time.Minute)}
	case "/api/v1/worker/report":
		f.reports++
		report := in.(workerqueue.Report)
		if f.reports > 1 && report != f.lastReport {
			return errors.New("different report retried")
		}
		f.lastReport = report
		if f.reports == 1 && f.firstReportErr != nil {
			return f.firstReportErr
		}
		v, err := workerqueue.Validate(f.a)
		if err != nil {
			return err
		}
		subject := f.Subject()
		if f.foreign {
			subject = "other"
		}
		result = workerqueue.Receipt{Token: f.a.Token, Worker: subject, Kind: workerqueue.Validated, Validation: v, ReceivedAt: time.Now()}
	default:
		return errors.New("unexpected request")
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
func newTransport(t *testing.T) *fakeTransport {
	t.Helper()
	task := core.TaskContract{ID: "task", WorkItemID: "work", Repository: "repo", BaseCommit: strings.Repeat("a", 40), AcceptanceCriteria: []string{"pass"}}
	td, _ := task.Digest()
	input := core.RunInputManifest{RunID: "run", TaskContractDigest: td, RuntimeProfile: "codex", ToolProfile: "tools", WorkerProfile: "worker/ubuntu", PolicyProfile: "policy"}
	d, _ := input.Digest()
	intent := workerqueue.Intent{RunID: "run", TaskDigest: td, InputDigest: d, ExecutionEpoch: 1}
	id, _ := intent.Digest()
	return &fakeTransport{a: workerqueue.Assignment{Token: workerqueue.Token{InboxID: 1, Generation: 1, Profile: "worker/ubuntu"}, Intent: intent, IntentDigest: id, Input: input, Task: task}}
}
func TestLostReportResponseRetriesOnlyIdenticalAcknowledgement(t *testing.T) {
	f := newTransport(t)
	f.firstReportErr = errors.New("response lost after commit")
	r, err := Once(context.Background(), f, "worker/ubuntu")
	if err != nil || r == nil {
		t.Fatalf("receipt retry: %v", err)
	}
	if f.claims != 1 || f.renewals != 1 || f.reports != 2 || r.Validation.ExecutionStarted {
		t.Fatalf("duplicate work or authority inflation: %#v", f)
	}
}
func TestBadAssignmentsAndAuthorityRejectionsNeverReportSuccess(t *testing.T) {
	t.Run("corrupt input", func(t *testing.T) {
		f := newTransport(t)
		f.a.Input.ToolProfile = "other"
		if _, err := Once(context.Background(), f, "worker/ubuntu"); err == nil || f.renewals != 0 || f.reports != 0 {
			t.Fatal("corrupt assignment reached report")
		}
	})
	t.Run("expired renewal", func(t *testing.T) {
		f := newTransport(t)
		f.renewErr = &controlclient.HTTPError{Status: 409}
		if _, err := Once(context.Background(), f, "worker/ubuntu"); err == nil || f.reports != 0 {
			t.Fatal("expired claim reported")
		}
	})
	t.Run("rejected report", func(t *testing.T) {
		f := newTransport(t)
		f.firstReportErr = &controlclient.HTTPError{Status: 403}
		if _, err := Once(context.Background(), f, "worker/ubuntu"); err == nil || f.reports != 1 {
			t.Fatal("authorization rejection retried")
		}
	})
	t.Run("foreign receipt", func(t *testing.T) {
		f := newTransport(t)
		f.foreign = true
		if _, err := Once(context.Background(), f, "worker/ubuntu"); err == nil {
			t.Fatal("foreign receipt accepted")
		}
	})
}
