package core

import (
	"sort"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

type WorkState string

const (
	WorkDraft     WorkState = "DRAFT"
	WorkReady     WorkState = "READY"
	WorkExecuting WorkState = "EXECUTING"
	WorkVerifying WorkState = "VERIFYING"
	WorkReviewing WorkState = "REVIEWING"
	WorkClosed    WorkState = "CLOSED"
	WorkCancelled WorkState = "CANCELLED"
)

type WorkItem struct {
	ID                       string    `json:"work_item_id"`
	Title                    string    `json:"title"`
	SourceRef                string    `json:"source_ref,omitempty"`
	HumanOwner               string    `json:"human_owner"`
	TargetID                 string    `json:"target_id,omitempty"`
	AssuranceClass           string    `json:"assurance_class,omitempty"`
	ActiveTaskContractDigest string    `json:"active_task_contract_digest,omitempty"`
	ActiveRunID              string    `json:"active_run_id,omitempty"`
	State                    WorkState `json:"state"`
	Version                  uint64    `json:"version"`
	CreatedAt                time.Time `json:"created_at"`
}

type TaskContract struct {
	ID                     string   `json:"task_contract_id"`
	WorkItemID             string   `json:"work_item_id"`
	TaskType               string   `json:"task_type"`
	CapabilityIDs          []string `json:"capability_ids"`
	SkillIDs               []string `json:"skill_ids"`
	Repository             string   `json:"repository"`
	BaseCommit             string   `json:"base_commit"`
	TargetID               string   `json:"target_id,omitempty"`
	AcceptanceCriteria     []string `json:"acceptance_criteria"`
	AllowedActions         []string `json:"allowed_actions,omitempty"`
	ExpectedOutputs        []string `json:"expected_outputs,omitempty"`
	VerificationPlanID     string   `json:"verification_plan_id"`
	VerificationPlanDigest string   `json:"verification_plan_digest"`
	Revision               uint64   `json:"revision"`
}

func (t TaskContract) Digest() (string, error) {
	return canonical.Digest(t)
}

type RunInputManifest struct {
	RunID              string   `json:"run_id"`
	TaskContractDigest string   `json:"task_contract_digest"`
	ContextRefs        []string `json:"context_refs,omitempty"`
	RuntimeProfile     string   `json:"runtime_profile"`
	ToolProfile        string   `json:"tool_profile"`
	WorkerProfile      string   `json:"worker_profile"`
	PolicyProfile      string   `json:"policy_profile"`
}

func (m RunInputManifest) Digest() (string, error) {
	return canonical.Digest(m)
}

type ArtifactRef struct {
	ID        string `json:"artifact_id"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type,omitempty"`
	Locator   string `json:"locator,omitempty"`
}

type EvidenceRef struct {
	ID                string   `json:"evidence_id"`
	DeliveryReceiptID string   `json:"delivery_receipt_id"`
	SubjectDigest     string   `json:"subject_digest"`
	Issuer            string   `json:"issuer"`
	Procedure         string   `json:"procedure"`
	Result            string   `json:"result"`
	ArtifactRefs      []string `json:"artifact_refs,omitempty"`
	Applicable        bool     `json:"applicable"`
}

type DeliveryReceipt struct {
	ID                 string        `json:"delivery_receipt_id"`
	WorkItemID         string        `json:"work_item_id"`
	TaskContractDigest string        `json:"task_contract_digest"`
	RunID              string        `json:"run_id"`
	TargetID           string        `json:"target_id,omitempty"`
	BaseCommit         string        `json:"base_commit"`
	ResultCommit       string        `json:"result_commit,omitempty"`
	SubjectDigest      string        `json:"subject_digest"`
	Artifacts          []ArtifactRef `json:"artifacts,omitempty"`
	KnownLimits        []string      `json:"known_limits,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
}

type deliverySubjectArtifact struct {
	ID     string `json:"artifact_id"`
	Digest string `json:"digest"`
}

type deliverySubject struct {
	TaskContractDigest string                    `json:"task_contract_digest"`
	RunID              string                    `json:"run_id"`
	TargetID           string                    `json:"target_id,omitempty"`
	BaseCommit         string                    `json:"base_commit"`
	ResultCommit       string                    `json:"result_commit,omitempty"`
	Artifacts          []deliverySubjectArtifact `json:"artifacts,omitempty"`
}

func (d DeliveryReceipt) CalculateSubjectDigest() (string, error) {
	subject := deliverySubject{
		TaskContractDigest: d.TaskContractDigest,
		RunID:              d.RunID,
		TargetID:           d.TargetID,
		BaseCommit:         d.BaseCommit,
		ResultCommit:       d.ResultCommit,
	}
	for _, artifact := range d.Artifacts {
		subject.Artifacts = append(subject.Artifacts, deliverySubjectArtifact{
			ID:     artifact.ID,
			Digest: artifact.Digest,
		})
	}
	sort.Slice(subject.Artifacts, func(i, j int) bool {
		if subject.Artifacts[i].ID == subject.Artifacts[j].ID {
			return subject.Artifacts[i].Digest < subject.Artifacts[j].Digest
		}
		return subject.Artifacts[i].ID < subject.Artifacts[j].ID
	})
	return canonical.Digest(subject)
}

type ClosureReceipt struct {
	ID                   string    `json:"closure_receipt_id"`
	WorkItemID           string    `json:"work_item_id"`
	TaskContractDigest   string    `json:"task_contract_digest"`
	RunID                string    `json:"run_id"`
	DeliveryReceiptID    string    `json:"delivery_receipt_id"`
	VerificationReportID string    `json:"verification_report_id"`
	ReviewReportID       string    `json:"review_report_id,omitempty"`
	SubjectDigest        string    `json:"subject_digest"`
	Result               string    `json:"result"`
	CreatedAt            time.Time `json:"created_at"`
}
