package githubpublish

import (
	"context"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
)

func boundPublicationReceipt(plan Plan, outcome string) PublicationReceipt {
	return PublicationReceipt{
		Version: 1, Repository: plan.Repository, BaseRef: plan.BaseRef,
		BaseCommit: plan.BaseCommit, Branch: plan.Branch,
		ResultCommit: plan.ResultCommit, PullRequestNumber: 17,
		PullRequestURL: "https://github.com/" + plan.Repository + "/pull/17",
		PullRequestState: "open", PublicationOutcome: outcome,
	}
}

func TestPublicationReceiptRejectsNonCanonicalPullLinks(t *testing.T) {
	plan, _ := historicalPullProbePlan(t)
	for _, tc := range []struct {
		name string
		link string
	}{
		{"different-pr", "https://github.com/" + plan.Repository + "/pull/18"},
		{"different-repository", "https://github.com/other/repository/pull/17"},
		{"suffix-path", "https://github.com/" + plan.Repository + "/pull/17/files"},
		{"query", "https://github.com/" + plan.Repository + "/pull/17?tab=files"},
		{"fragment", "https://github.com/" + plan.Repository + "/pull/17#discussion"},
		{"lookalike-number", "https://github.com/" + plan.Repository + "/pull/17abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receipt := boundPublicationReceipt(plan, "CREATED")
			receipt.PullRequestURL = tc.link
			if err := receipt.Validate(plan); err == nil {
				t.Fatalf("noncanonical PR link accepted: %q", tc.link)
			}
		})
	}
	if err := boundPublicationReceipt(plan, "CREATED").Validate(plan); err != nil {
		t.Fatalf("canonical PR link rejected: %v", err)
	}
}

func TestPullMatchRejectsInvalidURLBeforeAnyUpdate(t *testing.T) {
	plan, _ := historicalPullProbePlan(t)
	pr := pullRecord{
		Number: 17, HTMLURL: "https://github.com/" + plan.Repository + "/pull/17",
		State: "open",
	}
	pr.Head.Ref, pr.Head.SHA = plan.Branch, plan.ResultCommit
	pr.Base.Ref, pr.Base.SHA = plan.BaseRef, plan.BaseCommit
	if !pullMatches(pr, plan) {
		t.Fatal("canonical existing PR rejected")
	}
	for _, link := range []string{
		"https://github.com/" + plan.Repository + "/pull/18",
		"https://github.com/" + plan.Repository + "/pull/17/files",
		"https://github.com/other/repository/pull/17",
	} {
		pr.HTMLURL = link
		if pullMatches(pr, plan) {
			t.Fatalf("mismatched existing PR URL accepted before update: %q", link)
		}
	}
}

func TestProviderNeverConfirmsUnboundIndependentPublisherReceipts(t *testing.T) {
	state, config := publisherFixture(t)
	initial, err := New(config, state, &publisherRemote{})
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := initial.derive(context.Background(), state.run.ID, state.run.CurrentEpoch, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*PublicationReceipt)
	}{
		{"wrong-result", func(r *PublicationReceipt) { r.ResultCommit = plan.BaseCommit }},
		{"wrong-repository", func(r *PublicationReceipt) { r.Repository = "other/repository" }},
		{"wrong-branch", func(r *PublicationReceipt) { r.Branch = "other/branch" }},
		{"wrong-pr-url", func(r *PublicationReceipt) { r.PullRequestURL = "https://github.com/" + plan.Repository + "/pull/18" }},
		{"wrong-state", func(r *PublicationReceipt) { r.PullRequestState = "closed" }},
		{"wrong-outcome", func(r *PublicationReceipt) { r.PublicationOutcome = "UNKNOWN" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := boundPublicationReceipt(plan, "CREATED")
			tc.mutate(&bad)
			remote := &publisherRemote{receipt: bad}
			provider, err := New(config, state, remote)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Dispatch(context.Background(), action.Request{
				ID: "publisher-remote-boundary", RunID: state.run.ID,
				ExecutionEpoch: state.run.CurrentEpoch, Action: Action,
				RiskClass: action.ControlledMutation, Capability: Capability,
				ParametersDigest: state.status.Receipt.ResultDigest,
			})
			if err == nil || result.Outcome == action.DispatchConfirmed || remote.publishCalls != 1 {
				t.Fatalf("invalid external publication was confirmed: result=%#v err=%v calls=%d", result, err, remote.publishCalls)
			}

			badObservation := boundPublicationReceipt(plan, "OBSERVED")
			tc.mutate(&badObservation)
			observed := &publisherRemote{observation: ObserveResult{
				Outcome: ObservedConfirmed, Receipt: badObservation,
			}}
			reconciler, err := New(config, state, observed)
			if err != nil {
				t.Fatal(err)
			}
			settlement, err := reconciler.Reconcile(context.Background(), action.Operation{
				ID: "publisher-unknown", RunID: state.run.ID,
				ExecutionEpoch: state.run.CurrentEpoch, Action: Action,
				RiskClass: action.ControlledMutation, Capability: Capability,
			})
			if err != nil || settlement.Outcome != action.ReconcileManual ||
				settlement.ExternalRef != "" || !strings.Contains(settlement.ObservedState, "does not bind") ||
				observed.observeCalls != 1 || observed.publishCalls != 0 {
				t.Fatalf("invalid readback settled as authority: %#v err=%v observe=%d publish=%d",
					settlement, err, observed.observeCalls, observed.publishCalls)
			}
		})
	}
}
