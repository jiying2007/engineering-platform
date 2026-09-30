package production

import (
	"fmt"
	"sort"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const (
	TerminalPlanVersion = 1
	SLOReportVersion    = 1

	SLOStatusNonProviderComplete = "NON_PROVIDER_COMPLETE"
	SLOStatusComplete            = "COMPLETE"
)

type TerminalPlan struct {
	Version                  int      `json:"version"`
	FixtureID                string   `json:"fixture_id"`
	Repository               string   `json:"repository"`
	TaskType                 string   `json:"task_type"`
	TargetID                 string   `json:"target_id"`
	MarkerPath               string   `json:"marker_path"`
	MarkerBefore             string   `json:"marker_before"`
	MarkerAfter              string   `json:"marker_after"`
	MaxEngineeringModelTurns int      `json:"max_engineering_model_turns"`
	RequiredEvidence         []string `json:"required_evidence"`
	RequiredGates            []string `json:"required_gates"`
	HumanReviewRequired      bool     `json:"human_review_required"`
	ProviderLiveRequired     bool     `json:"provider_live_required"`
}

type TerminalPlanEnvelope struct {
	Plan       TerminalPlan `json:"plan"`
	PlanDigest string       `json:"plan_digest"`
}

func BuildTerminalPlan() (TerminalPlanEnvelope, error) {
	plan := TerminalPlan{
		Version:                  TerminalPlanVersion,
		FixtureID:                "production-terminal-maintenance-v1",
		Repository:               "jiying2007/engineering-platform",
		TaskType:                 "RELEASE",
		TargetID:                 "engineering-platform-production-qualification",
		MarkerPath:               "examples/production/terminal-maintenance/qualification.txt",
		MarkerBefore:             "PRE_LIVE_READY\n",
		MarkerAfter:              "TERMINAL_QUALIFIED\n",
		MaxEngineeringModelTurns: 1,
		RequiredEvidence: []string{
			"codex.core.execution.v1",
			"git.changed-tree.v1",
			"github.actions.trusted-ci.v1",
		},
		RequiredGates: []string{
			"production_preflight_ready",
			"operational_status_ready",
			"provider_live_qualified",
			"publisher_independent",
			"exact_pr_head_ci",
			"verification_pass",
			"independent_human_review_pass",
			"closure_created",
			"shutdown_restart_recovery_proven",
			"slo_report_complete",
		},
		HumanReviewRequired:  true,
		ProviderLiveRequired: true,
	}
	if err := plan.Validate(); err != nil {
		return TerminalPlanEnvelope{}, err
	}
	digest, err := canonical.Digest(plan)
	if err != nil {
		return TerminalPlanEnvelope{}, err
	}
	return TerminalPlanEnvelope{Plan: plan, PlanDigest: digest}, nil
}

func (p TerminalPlan) Validate() error {
	if p.Version != TerminalPlanVersion ||
		p.FixtureID != "production-terminal-maintenance-v1" ||
		p.Repository != "jiying2007/engineering-platform" ||
		p.TaskType != "RELEASE" ||
		p.TargetID != "engineering-platform-production-qualification" ||
		p.MarkerPath != "examples/production/terminal-maintenance/qualification.txt" ||
		p.MarkerBefore != "PRE_LIVE_READY\n" ||
		p.MarkerAfter != "TERMINAL_QUALIFIED\n" ||
		p.MarkerBefore == p.MarkerAfter ||
		p.MaxEngineeringModelTurns != 1 ||
		!p.HumanReviewRequired || !p.ProviderLiveRequired ||
		len(p.RequiredEvidence) != 3 || len(p.RequiredGates) != 10 {
		return fmt.Errorf("invalid production terminal acceptance plan")
	}
	expectedEvidence := []string{
		"codex.core.execution.v1",
		"git.changed-tree.v1",
		"github.actions.trusted-ci.v1",
	}
	for i := range expectedEvidence {
		if p.RequiredEvidence[i] != expectedEvidence[i] {
			return fmt.Errorf("production terminal evidence contract drift")
		}
	}
	expectedGates := []string{
		"production_preflight_ready",
		"operational_status_ready",
		"provider_live_qualified",
		"publisher_independent",
		"exact_pr_head_ci",
		"verification_pass",
		"independent_human_review_pass",
		"closure_created",
		"shutdown_restart_recovery_proven",
		"slo_report_complete",
	}
	for i := range expectedGates {
		if p.RequiredGates[i] != expectedGates[i] {
			return fmt.Errorf("production terminal gate contract drift")
		}
	}
	return nil
}

type SLOObservation struct {
	Name         string  `json:"name"`
	SamplesMS    []int64 `json:"samples_ms"`
	SourceDigest string  `json:"source_digest"`
}

type SLOMeasurement struct {
	Name         string `json:"name"`
	SampleCount  int    `json:"sample_count"`
	MinMS        int64  `json:"min_ms"`
	P50MS        int64  `json:"p50_ms"`
	P95MS        int64  `json:"p95_ms"`
	MaxMS        int64  `json:"max_ms"`
	SourceDigest string `json:"source_digest"`
}

type SLOReport struct {
	Version         int              `json:"version"`
	Status          string           `json:"status"`
	ProviderPending bool             `json:"provider_pending"`
	Measurements    []SLOMeasurement `json:"measurements"`
}

type SLOReportEnvelope struct {
	Report       SLOReport `json:"report"`
	ReportDigest string    `json:"report_digest"`
}

var nonProviderSLOs = []string{
	"control_api_roundtrip",
	"worker_admission",
	"worker_preparation",
	"engineering_run",
	"publication_ci_verification",
	"postgres_authority_restore",
}

const providerSLO = "provider_authentication"

func BuildSLOReport(observations []SLOObservation) (SLOReportEnvelope, error) {
	byName := make(map[string]SLOObservation, len(observations))
	for _, observation := range observations {
		if _, exists := byName[observation.Name]; exists {
			return SLOReportEnvelope{}, fmt.Errorf("duplicate SLO observation %q", observation.Name)
		}
		if !validSLOName(observation.Name) ||
			!canonical.ValidDigest(observation.SourceDigest) ||
			len(observation.SamplesMS) == 0 || len(observation.SamplesMS) > 1024 {
			return SLOReportEnvelope{}, fmt.Errorf("invalid SLO observation %q", observation.Name)
		}
		for _, sample := range observation.SamplesMS {
			if sample <= 0 || sample > 24*60*60*1000 {
				return SLOReportEnvelope{}, fmt.Errorf("invalid SLO sample for %q", observation.Name)
			}
		}
		byName[observation.Name] = observation
	}

	report := SLOReport{Version: SLOReportVersion, Status: SLOStatusNonProviderComplete, ProviderPending: true}
	order := append(append([]string{}, nonProviderSLOs...), providerSLO)
	for _, name := range order {
		observation, exists := byName[name]
		if !exists {
			if name == providerSLO {
				continue
			}
			return SLOReportEnvelope{}, fmt.Errorf("missing required non-provider SLO observation %q", name)
		}
		report.Measurements = append(report.Measurements, summarizeSLO(observation))
	}
	if _, exists := byName[providerSLO]; exists {
		report.Status = SLOStatusComplete
		report.ProviderPending = false
	}
	if len(byName) != len(report.Measurements) {
		return SLOReportEnvelope{}, fmt.Errorf("unknown SLO observation name")
	}
	digest, err := canonical.Digest(report)
	if err != nil {
		return SLOReportEnvelope{}, err
	}
	return SLOReportEnvelope{Report: report, ReportDigest: digest}, nil
}

func validSLOName(name string) bool {
	if name == providerSLO {
		return true
	}
	for _, required := range nonProviderSLOs {
		if name == required {
			return true
		}
	}
	return false
}

func summarizeSLO(observation SLOObservation) SLOMeasurement {
	samples := append([]int64(nil), observation.SamplesMS...)
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return SLOMeasurement{
		Name: observation.Name, SampleCount: len(samples),
		MinMS: samples[0], P50MS: percentile(samples, 50), P95MS: percentile(samples, 95),
		MaxMS: samples[len(samples)-1], SourceDigest: observation.SourceDigest,
	}
}

func percentile(sorted []int64, percent int) int64 {
	index := (percent*len(sorted) + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(sorted) {
		index = len(sorted)
	}
	return sorted[index-1]
}
