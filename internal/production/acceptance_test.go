package production

import (
	"strings"
	"testing"
)

func TestTerminalPlanIsDeterministicAndRequiresHumanReview(t *testing.T) {
	first, err := BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	if first.PlanDigest != second.PlanDigest || first.PlanDigest == "" ||
		first.Plan.FixtureID != second.Plan.FixtureID ||
		!first.Plan.HumanReviewRequired || !first.Plan.ProviderLiveRequired ||
		!first.Plan.NoSilentProviderFallback || !first.Plan.NoAutomaticDBDowngrade ||
		first.Plan.DeploymentProfile != "canary-single-maintenance-fixture" ||
		first.Plan.DatabaseRollbackPolicy != "restore-authoritative-backup-and-reconcile" ||
		len(first.Plan.EmergencyStops) != 3 || len(first.Plan.RequiredGates) != 14 ||
		first.Plan.TaskType != "RELEASE" {
		t.Fatalf("unexpected terminal plan: %#v", first)
	}
}

func TestTerminalPlanRejectsWeakenedRolloutAndEmergencyContract(t *testing.T) {
	envelope, err := BuildTerminalPlan()
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*TerminalPlan){
		func(p *TerminalPlan) { p.DeploymentProfile = "all-at-once" },
		func(p *TerminalPlan) { p.DatabaseRollbackPolicy = "auto-downgrade" },
		func(p *TerminalPlan) { p.EmergencyStops = p.EmergencyStops[:2] },
		func(p *TerminalPlan) { p.NoSilentProviderFallback = false },
		func(p *TerminalPlan) { p.NoAutomaticDBDowngrade = false },
		func(p *TerminalPlan) { p.RequiredGates = p.RequiredGates[:10] },
	}
	for i, mutate := range mutations {
		bad := envelope.Plan
		bad.EmergencyStops = append([]string(nil), envelope.Plan.EmergencyStops...)
		bad.RequiredGates = append([]string(nil), envelope.Plan.RequiredGates...)
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("weakened terminal rollout contract accepted", i)
		}
	}
}

func TestSLOReportAllowsProviderPendingButRequiresAllInternalMeasurements(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	observations := make([]SLOObservation, 0, len(nonProviderSLOs))
	for i, name := range nonProviderSLOs {
		observations = append(observations, SLOObservation{
			Name: name, SamplesMS: []int64{int64(i + 1), int64((i + 1) * 2)},
			SourceDigest: digest,
		})
	}
	envelope, err := BuildSLOReport(observations)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Report.Status != SLOStatusUnverified ||
		envelope.Report.ProviderMeasurementPresent ||
		len(envelope.Report.Measurements) != len(nonProviderSLOs) {
		t.Fatalf("unexpected provider-pending report: %#v", envelope)
	}

	observations = append(observations, SLOObservation{
		Name: providerSLO, SamplesMS: []int64{7, 9, 11}, SourceDigest: digest,
	})
	envelope, err = BuildSLOReport(observations)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Report.Status != SLOStatusUnverified || !envelope.Report.ProviderMeasurementPresent {
		t.Fatalf("provider measurement did not complete report: %#v", envelope)
	}
}

func TestSLOReportFailsClosedOnMissingOrUnknownObservation(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	if _, err := BuildSLOReport([]SLOObservation{{
		Name: "control_api_roundtrip", SamplesMS: []int64{1}, SourceDigest: digest,
	}}); err == nil {
		t.Fatal("incomplete internal SLO report accepted")
	}
	observations := make([]SLOObservation, 0, len(nonProviderSLOs)+1)
	for _, name := range nonProviderSLOs {
		observations = append(observations, SLOObservation{
			Name: name, SamplesMS: []int64{1}, SourceDigest: digest,
		})
	}
	observations = append(observations, SLOObservation{
		Name: "made_up_metric", SamplesMS: []int64{1}, SourceDigest: digest,
	})
	if _, err := BuildSLOReport(observations); err == nil {
		t.Fatal("unknown SLO observation accepted")
	}
}
