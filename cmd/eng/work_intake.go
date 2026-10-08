package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/routing"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type workIntakeSpec struct {
	Version          int                    `json:"version"`
	WorkItemID       string                 `json:"work_item_id"`
	TaskContractID   string                 `json:"task_contract_id"`
	RunID            string                 `json:"run_id"`
	AttemptID        string                 `json:"attempt_id"`
	Title            string                 `json:"title"`
	SourceRef        string                 `json:"source_ref,omitempty"`
	AssuranceClass   string                 `json:"assurance_class,omitempty"`
	TaskType         string                 `json:"task_type"`
	Subsystem        string                 `json:"subsystem"`
	TargetContext    *routing.TargetContext `json:"target_context,omitempty"`
	AllowedActions   []string               `json:"allowed_actions,omitempty"`
	ExpectedOutputs  []string               `json:"expected_outputs,omitempty"`
	Material         material.Manifest      `json:"material"`
	VerificationPlan verification.Plan      `json:"verification_plan"`
	ContextRefs      []core.ContextRef      `json:"context_refs,omitempty"`
	RuntimeProfile   string                 `json:"runtime_profile"`
	ToolProfile      string                 `json:"tool_profile"`
	WorkerProfile    string                 `json:"worker_profile"`
	PolicyProfile    string                 `json:"policy_profile"`
}

type workIntakeReceipt struct {
	Version                int    `json:"version"`
	Status                 string `json:"status"`
	InputFileDigest        string `json:"input_file_digest"`
	HumanOwner             string `json:"human_owner"`
	WorkItemID             string `json:"work_item_id"`
	TaskContractDigest     string `json:"task_contract_digest"`
	RunID                  string `json:"run_id"`
	RunInputManifestDigest string `json:"run_input_manifest_digest"`
	ExecutionEpoch         uint64 `json:"execution_epoch"`
	ContextCount           int    `json:"context_count"`
	WorkerExecutionStarted bool   `json:"worker_execution_started"`
	ModelTurnExecuted      bool   `json:"model_turn_executed"`
	ProductionQualified    bool   `json:"production_qualified"`
}

type workIntakeError struct {
	Phase       string
	WorkCreated bool
	TaskCreated bool
	Cause       error
}

func (e *workIntakeError) Error() string {
	return fmt.Sprintf("work-intake phase %s failed (work_created=%t task_created=%t; inspect exact IDs before retry): %v",
		e.Phase, e.WorkCreated, e.TaskCreated, e.Cause)
}
func (e *workIntakeError) Unwrap() error { return e.Cause }

func boundedIntakeValue(v string, max int) bool {
	return v != "" && len(v) <= max && strings.TrimSpace(v) == v && !strings.ContainsAny(v, "\x00\r\n")
}

func uniqueIntakeValues(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if !boundedIntakeValue(value, 256) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (s workIntakeSpec) validate() (routing.Route, material.Result, error) {
	for _, id := range []string{s.WorkItemID, s.TaskContractID, s.RunID, s.AttemptID} {
		if !controlPathID.MatchString(id) {
			return routing.Route{}, material.Result{}, fmt.Errorf("path-safe Work/Task/Run identities required")
		}
	}
	if s.Version != 1 ||
		!boundedIntakeValue(s.Title, 512) ||
		len(s.SourceRef) > 1024 || strings.ContainsAny(s.SourceRef, "\x00\r\n") ||
		!boundedIntakeValue(s.TaskType, 64) || !boundedIntakeValue(s.Subsystem, 128) ||
		!uniqueIntakeValues(s.AllowedActions) || !uniqueIntakeValues(s.ExpectedOutputs) ||
		!boundedIntakeValue(s.RuntimeProfile, 256) || !boundedIntakeValue(s.ToolProfile, 256) ||
		!boundedIntakeValue(s.WorkerProfile, 256) || !boundedIntakeValue(s.PolicyProfile, 256) {
		return routing.Route{}, material.Result{}, fmt.Errorf("invalid bounded work-intake field")
	}
	if s.Material.TaskType != "" {
		return routing.Route{}, material.Result{}, fmt.Errorf("material task_type must be omitted; intake task_type is authoritative")
	}
	if err := core.ValidateContextRefs(s.ContextRefs); err != nil {
		return routing.Route{}, material.Result{}, err
	}
	m := s.Material
	m.TaskType = s.TaskType
	readiness := material.Evaluate(m)
	if readiness.Status == material.Blocked {
		return routing.Route{}, readiness, readiness.Error()
	}
	if !verification.ValidatePlan(s.VerificationPlan, m.AcceptanceCriteria) {
		return routing.Route{}, readiness, fmt.Errorf("verification plan does not cover acceptance criteria exactly")
	}
	if s.TargetContext != nil && s.TargetContext.TargetID != s.Material.TargetID {
		return routing.Route{}, readiness, fmt.Errorf("target_context target_id must match material target_id")
	}
	route, err := routing.ResolveForTarget(s.TaskType, s.Subsystem, s.TargetContext)
	if err != nil {
		return routing.Route{}, readiness, err
	}
	return route, readiness, nil
}

func targetPlatform(ctx *routing.TargetContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.Platform
}

func loadWorkIntake(path string) (workIntakeSpec, string, error) {
	var spec workIntakeSpec
	raw, err := access.ReadConfiguration(path, false)
	if err != nil {
		return spec, "", err
	}
	if len(raw) == 0 || len(raw) > controlclient.MaxRequest {
		return spec, "", fmt.Errorf("bounded owner-controlled intake file required")
	}
	if err := strictjson.Decode(raw, &spec); err != nil {
		return spec, "", err
	}
	if _, _, err := spec.validate(); err != nil {
		return spec, "", err
	}
	return spec, canonical.BytesDigest(raw), nil
}

type workIntakeTaskRequest struct {
	Contract         core.TaskContract      `json:"contract"`
	Material         material.Manifest      `json:"material"`
	Subsystem        string                 `json:"subsystem,omitempty"`
	TargetContext    *routing.TargetContext `json:"target_context,omitempty"`
	VerificationPlan verification.Plan      `json:"verification_plan"`
}
type workIntakeTaskResponse struct {
	Contract  core.TaskContract `json:"contract"`
	Digest    string            `json:"digest"`
	Readiness material.Result   `json:"readiness"`
}
type workIntakeRunRequest struct {
	RunID              string                `json:"run_id"`
	TaskContractDigest string                `json:"task_contract_digest"`
	AttemptID          string                `json:"attempt_id"`
	RunInput           core.RunInputManifest `json:"run_input"`
}
type workIntakeRunResponse struct {
	Run      run.Run               `json:"run"`
	Attempt  run.Attempt           `json:"attempt"`
	Session  session.Session       `json:"session"`
	RunInput core.RunInputManifest `json:"run_input"`
}

func executeWorkIntake(ctx context.Context, client *controlclient.Client, spec workIntakeSpec, inputFileDigest string) (workIntakeReceipt, error) {
	var zero workIntakeReceipt
	if client == nil || !canonical.ValidDigest(inputFileDigest) {
		return zero, fmt.Errorf("authenticated client and exact intake digest required")
	}
	route, readiness, err := spec.validate()
	if err != nil {
		return zero, err
	}
	// Validate exact selected Skill metadata before any Work/Task mutation.
	// No typed target means the historical Task contract remains unchanged.
	expectedSkillContractDigest := ""
	expectedGuidanceVersion := 0
	if spec.TargetContext != nil {
		expectedGuidanceVersion = 1
		expectedSkillContractDigest, err = embedded.RoutedSkillContractDigest(route.SkillIDs)
		if err != nil {
			return zero, fmt.Errorf("selected Skill contract invalid: %w", err)
		}
	}
	work := core.WorkItem{
		ID: spec.WorkItemID, Title: spec.Title, SourceRef: spec.SourceRef,
		HumanOwner: client.Subject(), TargetID: spec.Material.TargetID, AssuranceClass: spec.AssuranceClass,
	}
	var workResponse core.WorkItem
	if err := client.Call(ctx, http.MethodPost, "/api/v1/work-items", work, &workResponse); err != nil {
		return zero, &workIntakeError{Phase: "WORK", Cause: err}
	}
	if workResponse.ID != work.ID || workResponse.Title != work.Title ||
		workResponse.HumanOwner != client.Subject() || workResponse.State != core.WorkDraft ||
		workResponse.Version != 1 || workResponse.CreatedAt.IsZero() {
		return zero, &workIntakeError{Phase: "WORK_READBACK", WorkCreated: true, Cause: fmt.Errorf("work readback mismatch")}
	}

	m := spec.Material
	m.TaskType = spec.TaskType
	request := workIntakeTaskRequest{
		Contract: core.TaskContract{
			ID: spec.TaskContractID, WorkItemID: spec.WorkItemID, TaskType: spec.TaskType,
			AllowedActions:  append([]string(nil), spec.AllowedActions...),
			ExpectedOutputs: append([]string(nil), spec.ExpectedOutputs...),
		},
		Material: m, Subsystem: spec.Subsystem, TargetContext: spec.TargetContext, VerificationPlan: spec.VerificationPlan,
	}
	var taskResponse workIntakeTaskResponse
	if err := client.Call(ctx, http.MethodPost, "/api/v1/task-contracts", request, &taskResponse); err != nil {
		return zero, &workIntakeError{Phase: "TASK", WorkCreated: true, Cause: err}
	}
	taskDigest, digestErr := taskResponse.Contract.Digest()
	if digestErr != nil || taskDigest != taskResponse.Digest ||
		taskResponse.Contract.ID != spec.TaskContractID ||
		taskResponse.Contract.WorkItemID != spec.WorkItemID ||
		taskResponse.Contract.Repository != m.Repository ||
		taskResponse.Contract.BaseCommit != m.BaseCommit ||
		taskResponse.Contract.TargetID != m.TargetID ||
		taskResponse.Contract.TargetPlatform != targetPlatform(spec.TargetContext) ||
		taskResponse.Contract.SkillContractDigest != expectedSkillContractDigest ||
		taskResponse.Contract.SkillGuidanceVersion != expectedGuidanceVersion ||
		!reflect.DeepEqual(taskResponse.Contract.AcceptanceCriteria, m.AcceptanceCriteria) ||
		!reflect.DeepEqual(taskResponse.Contract.CapabilityIDs, route.CapabilityIDs) ||
		!reflect.DeepEqual(taskResponse.Contract.SkillIDs, route.SkillIDs) ||
		!reflect.DeepEqual(taskResponse.Readiness, readiness) {
		return zero, &workIntakeError{Phase: "TASK_READBACK", WorkCreated: true, TaskCreated: true, Cause: fmt.Errorf("task readback mismatch")}
	}

	input := core.RunInputManifest{
		RunID: spec.RunID, TaskContractDigest: taskResponse.Digest,
		ContextRefs:    append([]core.ContextRef(nil), spec.ContextRefs...),
		RuntimeProfile: spec.RuntimeProfile, ToolProfile: spec.ToolProfile,
		WorkerProfile: spec.WorkerProfile, PolicyProfile: spec.PolicyProfile,
	}
	inputDigest, err := input.Digest()
	if err != nil {
		return zero, &workIntakeError{Phase: "RUN_INPUT", WorkCreated: true, TaskCreated: true, Cause: err}
	}
	var runResponse workIntakeRunResponse
	if err := client.Call(ctx, http.MethodPost, "/api/v1/runs", workIntakeRunRequest{
		RunID: spec.RunID, TaskContractDigest: taskResponse.Digest,
		AttemptID: spec.AttemptID, RunInput: input,
	}, &runResponse); err != nil {
		return zero, &workIntakeError{Phase: "RUN", WorkCreated: true, TaskCreated: true, Cause: err}
	}
	if runResponse.Run.ID != spec.RunID ||
		runResponse.Run.TaskContractDigest != taskResponse.Digest ||
		runResponse.Run.RunInputManifestDigest != inputDigest ||
		runResponse.Run.State != run.Running ||
		runResponse.Run.CurrentEpoch != 1 ||
		runResponse.Run.CurrentAttemptID != spec.AttemptID ||
		runResponse.Attempt.ID != spec.AttemptID || runResponse.Attempt.Epoch != 1 ||
		runResponse.Session.RunID != spec.RunID || runResponse.Session.ExecutionEpoch != 1 ||
		runResponse.Session.Owner != session.Runtime ||
		!reflect.DeepEqual(runResponse.RunInput, input) {
		return zero, &workIntakeError{Phase: "RUN_READBACK", WorkCreated: true, TaskCreated: true, Cause: fmt.Errorf("run readback mismatch")}
	}
	return workIntakeReceipt{
		Version: 1, Status: "WORK_TASK_RUN_CREATED",
		InputFileDigest: inputFileDigest, HumanOwner: client.Subject(),
		WorkItemID: spec.WorkItemID, TaskContractDigest: taskResponse.Digest,
		RunID: spec.RunID, RunInputManifestDigest: inputDigest,
		ExecutionEpoch: 1, ContextCount: len(input.ContextRefs),
	}, nil
}

func workIntake(args []string) error {
	if len(args) != 2 || args[0] != "submit" {
		return fmt.Errorf("usage: eng work-intake submit INTAKE.json")
	}
	spec, digest, err := loadWorkIntake(args[1])
	if err != nil {
		return err
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	receipt, err := executeWorkIntake(ctx, client, spec, digest)
	if err != nil {
		return err
	}
	printJSON(receipt)
	return nil
}
