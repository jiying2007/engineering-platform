package githubpublish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
)

func TestProviderDispatchRejectsObservationOnlyReceipt(t *testing.T) {
	for _, outcome := range []string{"CREATED", "UPDATED", "EXISTING", "OBSERVED"} {
		t.Run(outcome, func(t *testing.T) {
			state, config := publisherFixture(t)
			remote := &publisherRemote{observation: ObserveResult{Outcome: ObservedAbsent}}
			provider, err := New(config, state, remote)
			if err != nil {
				t.Fatal(err)
			}
			plan, _, err := provider.derive(context.Background(), state.run.ID, state.run.CurrentEpoch, "", false)
			if err != nil {
				t.Fatal(err)
			}
			remote.receipt = boundPublicationReceipt(plan, outcome)
			repository := action.NewMemoryRepository()
			service := action.NewService(action.AllowCapabilities{Capability: true}, publicationOutcomeGuard{}, provider, repository)
			request := action.Request{
				ID: "publication-outcome", RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch,
				Action: Action, RiskClass: action.ControlledMutation, Capability: Capability,
				ParametersDigest: state.status.Receipt.ResultDigest, IdempotencyKey: "publication-outcome-once",
			}
			result, err := service.Execute(context.Background(), request)
			want := action.Confirmed
			if outcome == "OBSERVED" {
				want = action.Unknown
			}
			if err != nil || result.Result != string(want) || remote.publishCalls != 1 {
				t.Fatalf("publication outcome %s gained wrong authority: result=%#v err=%v calls=%d", outcome, result, err, remote.publishCalls)
			}
			stored, err := repository.GetContext(context.Background(), request.ID)
			if err != nil || stored.State != want {
				t.Fatalf("wrong durable Action result: %#v %v", stored, err)
			}
			if outcome == "OBSERVED" && (stored.ExternalRef != "" || stored.ObservedState != "") {
				t.Fatal("observation-only response retained as original publication success", stored)
			}
			if repeated, err := service.Execute(context.Background(), request); err != nil || repeated.Result != string(want) || remote.publishCalls != 1 {
				t.Fatalf("exact repeat replayed publication: %#v %v calls=%d", repeated, err, remote.publishCalls)
			}
			if outcome == "OBSERVED" {
				observed, err := service.Reconcile(context.Background(), request.ID)
				if err != nil || observed.Result != string(action.Manual) || remote.publishCalls != 1 || remote.observeCalls != 1 {
					t.Fatalf("ordinary absence observation gained terminal/replay authority: %#v %v", observed, err)
				}
			}
		})
	}
}

type publicationOutcomeGuard struct{}

func (publicationOutcomeGuard) CheckRunEpoch(context.Context, string, uint64) error { return nil }
func (publicationOutcomeGuard) CheckRecoveryEpoch(context.Context, uint64, action.RiskClass) error {
	return nil
}

func TestPublisherRemoteClientBindsReceiptToResponsePurpose(t *testing.T) {
	plan, _ := historicalPullProbePlan(t)
	for _, outcome := range []string{"CREATED", "UPDATED", "EXISTING", "OBSERVED"} {
		for _, observe := range []bool{false, true} {
			name := "publish/" + outcome
			if observe {
				name = "observe/" + outcome
			}
			t.Run(name, func(t *testing.T) {
				receipt := boundPublicationReceipt(plan, outcome)
				var payload any = receipt
				if observe {
					payload = ObserveResult{Outcome: ObservedConfirmed, Receipt: receipt}
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				body := &responseProbeBody{reader: strings.NewReader(string(raw))}
				calls := 0
				client := remoteResponseProbe(body, http.StatusOK, remoteResponseHeader(), &calls)
				if observe {
					got, err := client.Observe(context.Background(), plan)
					if outcome == "OBSERVED" {
						if err != nil || got != (ObserveResult{Outcome: ObservedConfirmed, Receipt: receipt}) {
							t.Fatalf("valid observation rejected: %#v %v", got, err)
						}
					} else if err == nil || got != (ObserveResult{}) {
						t.Fatalf("publication result accepted as an observation: %#v %v", got, err)
					}
				} else {
					got, err := client.Publish(context.Background(), plan, "original-bundle", testDispatchIdentity(plan))
					if outcome != "OBSERVED" {
						if err != nil || got != receipt {
							t.Fatalf("valid publication rejected: %#v %v", got, err)
						}
					} else if err == nil || got != (PublicationReceipt{}) {
						t.Fatalf("observation accepted as original publication: %#v %v", got, err)
					}
				}
				if calls != 1 || body.closed != 1 {
					t.Fatal("outcome validation repeated the exchange or lost body close", calls, body.closed)
				}
			})
		}
	}
}

// The existing protocol fixture supplies synthetic original Git proofs. These
// tests verify response-purpose binding, not live upstream publication evidence.
type publicationOutcomeRemote struct {
	*serviceRemote
	outcome string
}

func (r *publicationOutcomeRemote) Publish(ctx context.Context, plan Plan, bundle string, identity DispatchIdentity) (PublicationReceipt, error) {
	receipt, err := r.serviceRemote.Publish(ctx, plan, bundle, identity)
	receipt.PublicationOutcome = r.outcome
	return receipt, err
}

func (r *publicationOutcomeRemote) Observe(_ context.Context, plan Plan) (ObserveResult, error) {
	r.observeCalls++
	return ObserveResult{Outcome: ObservedConfirmed, Receipt: boundPublicationReceipt(plan, r.outcome)}, nil
}

func TestPublisherServiceBindsReceiptToResponsePurpose(t *testing.T) {
	for _, outcome := range []string{"CREATED", "UPDATED", "EXISTING", "OBSERVED"} {
		t.Run("publish/"+outcome, func(t *testing.T) {
			service, plan := publisherServiceFixture(t)
			remote := &publicationOutcomeRemote{serviceRemote: service.remote.(*serviceRemote), outcome: outcome}
			service.remote = remote
			identity := testDispatchIdentity(plan)
			response := evidenceCall(t, service, plan, identity, "/v1/publish")
			start, completion, err := service.journal.completed(context.Background(), identity, plan)
			if outcome != "OBSERVED" {
				if response.Code != http.StatusOK || err != nil || completion.ValidateForRecovery(start) != nil {
					t.Fatalf("valid original publication lost completion: HTTP %d %v", response.Code, err)
				}
			} else {
				if response.Code >= 200 && response.Code < 300 || err == nil {
					t.Fatalf("observation-only return minted original completion: HTTP %d %v", response.Code, err)
				}
				if sealed := evidenceCall(t, service, plan, identity, "/v1/dispatch-no-invocation"); sealed.Code != http.StatusConflict {
					t.Fatal("rejected outcome was relabelled as no invocation", sealed.Code)
				}
			}
			if repeated := evidenceCall(t, service, plan, identity, "/v1/publish"); repeated.Code != http.StatusConflict || remote.publishCalls != 1 || remote.observeCalls != 0 {
				t.Fatal("outcome validation allowed replay or fabricated external absence", repeated.Code, remote.publishCalls, remote.observeCalls)
			}
		})
		t.Run("observe/"+outcome, func(t *testing.T) {
			service, plan := publisherServiceFixture(t)
			remote := &publicationOutcomeRemote{serviceRemote: service.remote.(*serviceRemote), outcome: outcome}
			service.remote = remote
			raw, err := json.Marshal(observeRequest{Plan: plan})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			service.Handler().ServeHTTP(response, authorizedPublisherRequest(t, http.MethodPost, "/v1/observe", raw))
			if outcome == "OBSERVED" {
				if response.Code != http.StatusOK {
					t.Fatal("valid observation rejected", response.Code)
				}
			} else if response.Code >= 200 && response.Code < 300 {
				t.Fatal("publication result presented as fresh observation", response.Code)
			}
			if remote.publishCalls != 0 || remote.observeCalls != 1 {
				t.Fatal("observation changed external call authority", remote.publishCalls, remote.observeCalls)
			}
		})
	}
}
