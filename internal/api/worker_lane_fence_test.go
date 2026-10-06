package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// Real mTLS/router tests with an explicitly inert, counting repository. They
// prove rejection before storage, not database receipts or source preparation.
type laneFenceRepository struct {
	store.Store
	claims, renews, inputReports, preparedReports, readiness atomic.Int64
}

func (s *laneFenceRepository) ClaimInput(context.Context, string, string) (*workerqueue.Assignment, error) {
	s.claims.Add(1)
	return nil, nil
}
func (s *laneFenceRepository) RenewInput(context.Context, string, workerqueue.Token) (time.Time, error) {
	s.renews.Add(1)
	return time.Now().Add(time.Minute), nil
}
func (s *laneFenceRepository) ReportInput(context.Context, string, workerqueue.Report) (workerqueue.Receipt, error) {
	s.inputReports.Add(1)
	return workerqueue.Receipt{}, nil
}
func (s *laneFenceRepository) GetInbox(context.Context, string) (workerqueue.Status, error) {
	return workerqueue.Status{}, nil
}
func (s *laneFenceRepository) PreparationReady(context.Context) error {
	s.readiness.Add(1)
	return nil
}
func (s *laneFenceRepository) ReportPrepared(context.Context, string, preparation.Report) (preparation.Receipt, error) {
	s.preparedReports.Add(1)
	return preparation.Receipt{}, nil
}
func (s *laneFenceRepository) GetPreparation(context.Context, string) (preparation.Receipt, error) {
	return preparation.Receipt{}, nil
}
func (s *laneFenceRepository) counts() [5]int64 {
	return [5]int64{s.claims.Load(), s.renews.Load(), s.inputReports.Load(), s.preparedReports.Load(), s.readiness.Load()}
}

func TestWorkerClaimLaneEnforcedAtAuthenticatedEndpoints(t *testing.T) {
	const admit = "urn:engineering-platform:worker:lane-admit"
	const prepare = "urn:engineering-platform:worker:lane-prepare"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: admit, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport}, WorkerProfiles: []string{"worker/admit"}},
		{Subject: prepare, Scope: "platform", Capabilities: []string{access.WorkerPoll, access.WorkerReport, access.WorkerPrepare}, WorkerProfiles: []string{"worker/prepare"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	repository := &laneFenceRepository{Store: store.NewMemory()}
	handler, err := NewAuthenticatedHandler(repository, nil, policy, AuthenticatedOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	cases := []struct {
		name, actor, path, body string
		status                  int
		delta                   [5]int64
	}{
		{"preparer cannot plain-claim its own route", prepare, "/claim", `{"worker_profile":"worker/prepare"}`, 403, [5]int64{}},
		{"preparer cannot plain-report its own token", prepare, "/report", `{"token":{"worker_profile":"worker/prepare"}}`, 403, [5]int64{}},
		{"malformed claim rejected before storage", prepare, "/claim", `{`, 400, [5]int64{}},
		{"malformed report rejected before storage", prepare, "/report", `{`, 400, [5]int64{}},
		{"admission cannot prepare", admit, "/prepare-claim", `{"worker_profile":"worker/admit"}`, 403, [5]int64{}},
		{"admission cannot report prepared", admit, "/prepared", `{}`, 403, [5]int64{}},
		{"admission normal claim", admit, "/claim", `{"worker_profile":"worker/admit"}`, 200, [5]int64{1, 0, 0, 0, 0}},
		{"preparation normal claim", prepare, "/prepare-claim", `{"worker_profile":"worker/prepare"}`, 200, [5]int64{1, 0, 0, 0, 1}},
		{"admission wrong profile", admit, "/claim", `{"worker_profile":"worker/prepare"}`, 403, [5]int64{}},
		{"preparation lease renewal preserved", prepare, "/renew", `{"worker_profile":"worker/prepare"}`, 200, [5]int64{0, 1, 0, 0, 0}},
		{"admission lease renewal preserved", admit, "/renew", `{"worker_profile":"worker/admit"}`, 200, [5]int64{0, 1, 0, 0, 0}},
		{"admission normal report", admit, "/report", `{"token":{"worker_profile":"worker/admit"}}`, 200, [5]int64{0, 0, 1, 0, 0}},
		{"preparation normal report", prepare, "/prepared", `{"input_report":{"token":{"worker_profile":"worker/prepare"}}}`, 200, [5]int64{0, 0, 0, 1, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := repository.counts()
			client := pki.Client(t, tc.actor)
			defer client.CloseIdleConnections()
			response, err := client.Post(server.URL+"/api/v1/worker"+tc.path, "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("status=%d want=%d", response.StatusCode, tc.status)
			}
			after := repository.counts()
			for i := range after {
				if after[i]-before[i] != tc.delta[i] {
					t.Fatalf("repository call delta=%v before=%v want=%v", after, before, tc.delta)
				}
			}
		})
	}
}
