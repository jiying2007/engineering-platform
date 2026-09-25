package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/routing"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type ActionGateway interface {
	Execute(context.Context, action.Request) (action.Receipt, error)
	Get(string) (action.Operation, error)
	Reconcile(context.Context, string) (action.Receipt, error)
}

type RecoveryProofStore interface {
	CreateRecoveryProof(context.Context, uint64, string) (recovery.Proof, error)
	GetRecoveryProof(context.Context, uint64) (recovery.Proof, error)
}

type Server struct {
	store   store.Store
	actions ActionGateway
	mux     *http.ServeMux
	now     func() time.Time
}

func NewServer(s store.Store) *Server {
	return newServer(s, nil)
}

func NewServerWithActionGateway(s store.Store, actions ActionGateway) *Server {
	return newServer(s, actions)
}

func newServer(s store.Store, actions ActionGateway) *Server {
	if s == nil {
		s = store.NewMemory()
	}
	server := &Server{
		store:   s,
		actions: actions,
		mux:     http.NewServeMux(),
		now:     func() time.Time { return time.Now().UTC() },
	}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("GET /api/v1/recovery", s.handleGetRecovery)
	s.mux.HandleFunc("POST /api/v1/recovery/begin", s.handleBeginRecovery)
	s.mux.HandleFunc("POST /api/v1/recovery/complete", s.handleCompleteRecovery)
	s.mux.HandleFunc("POST /api/v1/recovery/proofs", s.handleCreateRecoveryProof)
	s.mux.HandleFunc("GET /api/v1/recovery/proofs/{epoch}", s.handleGetRecoveryProof)
	s.mux.HandleFunc("POST /api/v1/work-items", s.handleCreateWork)
	s.mux.HandleFunc("GET /api/v1/work-items/{id}", s.handleGetWork)
	s.mux.HandleFunc("POST /api/v1/task-contracts", s.handleCreateTask)
	s.mux.HandleFunc("GET /api/v1/task-contracts/{id}", s.handleGetTask)
	s.mux.HandleFunc("POST /api/v1/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /api/v1/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/steer", s.handleSteer)
	s.mux.HandleFunc("GET /api/v1/steering/{id}", s.handleGetSteering)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/pause", s.handlePause)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/resume", s.handleResume)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/takeover", s.handleTakeover)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/complete", s.handleCompleteRun)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/checkpoints", s.handleCreateCheckpoint)
	s.mux.HandleFunc("GET /api/v1/checkpoints/{id}", s.handleGetCheckpoint)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/actions", s.handleCreateAction)
	s.mux.HandleFunc("GET /api/v1/actions/{id}", s.handleGetAction)
	s.mux.HandleFunc("POST /api/v1/actions/{id}/reconcile", s.handleReconcileAction)
	s.mux.HandleFunc("POST /api/v1/deliveries", s.handleCreateDelivery)
	s.mux.HandleFunc("GET /api/v1/deliveries/{id}", s.handleGetDelivery)
	s.mux.HandleFunc("POST /api/v1/evidence", s.handleCreateEvidence)
	s.mux.HandleFunc("GET /api/v1/evidence/{id}", s.handleGetEvidence)
	s.mux.HandleFunc("POST /api/v1/verifications", s.handleCreateVerification)
	s.mux.HandleFunc("GET /api/v1/verifications/{id}", s.handleGetVerification)
	s.mux.HandleFunc("POST /api/v1/reviews", s.handleCreateReview)
	s.mux.HandleFunc("GET /api/v1/reviews/{id}", s.handleGetReview)
	s.mux.HandleFunc("POST /api/v1/closures", s.handleCreateClosure)
	s.mux.HandleFunc("GET /api/v1/closures/{id}", s.handleGetClosure)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "engineering-control-plane",
	})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"capabilities": embedded.Capabilities(),
		"skills":       embedded.Skills(),
	})
}

func (s *Server) handleGetRecovery(w http.ResponseWriter, _ *http.Request) {
	state, err := s.store.GetRecovery()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type beginRecoveryRequest struct {
	ExpectedRecoveryEpoch uint64 `json:"expected_recovery_epoch"`
}

func (s *Server) handleBeginRecovery(w http.ResponseWriter, r *http.Request) {
	var req beginRecoveryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	state, err := s.store.BeginRecovery(req.ExpectedRecoveryEpoch)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict), errors.Is(err, recovery.ErrAlreadyRecovering):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type completeRecoveryRequest struct {
	RecoveryEpoch uint64 `json:"recovery_epoch"`
}

func (s *Server) handleCompleteRecovery(w http.ResponseWriter, r *http.Request) {
	var req completeRecoveryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RecoveryEpoch == 0 {
		writeError(w, http.StatusBadRequest, "recovery_epoch is required")
		return
	}
	state, err := s.store.CompleteRecovery(req.RecoveryEpoch, true)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict), errors.Is(err, recovery.ErrStaleEpoch):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, recovery.ErrReconciliationRequired):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type createRecoveryProofRequest struct {
	RecoveryEpoch uint64 `json:"recovery_epoch"`
	Reconciler    string `json:"reconciler"`
}

func (s *Server) handleCreateRecoveryProof(w http.ResponseWriter, r *http.Request) {
	var req createRecoveryProofRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RecoveryEpoch == 0 || req.Reconciler == "" {
		writeError(w, http.StatusBadRequest, "recovery_epoch and reconciler are required")
		return
	}
	proofs, ok := s.store.(RecoveryProofStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "durable recovery proof store is not configured")
		return
	}
	proof, err := proofs.CreateRecoveryProof(r.Context(), req.RecoveryEpoch, req.Reconciler)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, proof)
}

func (s *Server) handleGetRecoveryProof(w http.ResponseWriter, r *http.Request) {
	epoch, err := strconv.ParseUint(r.PathValue("epoch"), 10, 64)
	if err != nil || epoch == 0 {
		writeError(w, http.StatusBadRequest, "valid recovery epoch required")
		return
	}
	proofs, ok := s.store.(RecoveryProofStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "durable recovery proof store is not configured")
		return
	}
	proof, err := proofs.GetRecoveryProof(r.Context(), epoch)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proof)
}

func (s *Server) handleCreateWork(w http.ResponseWriter, r *http.Request) {
	var item core.WorkItem
	if !decodeJSON(w, r, &item) {
		return
	}
	if item.ID == "" || item.Title == "" || item.HumanOwner == "" {
		writeError(w, http.StatusBadRequest, "work_item_id, title and human_owner are required")
		return
	}
	if item.State == "" {
		item.State = core.WorkDraft
	}
	if item.Version == 0 {
		item.Version = 1
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = s.now()
	}
	if err := s.store.CreateWork(item); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleGetWork(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetWork(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type createTaskRequest struct {
	Contract         core.TaskContract `json:"contract"`
	Material         material.Manifest `json:"material"`
	Subsystem        string            `json:"subsystem,omitempty"`
	VerificationPlan verification.Plan `json:"verification_plan"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Contract.ID == "" || req.Contract.WorkItemID == "" || req.Contract.TaskType == "" {
		writeError(w, http.StatusBadRequest, "task_contract_id, work_item_id and task_type are required")
		return
	}
	work, err := s.store.GetWork(req.Contract.WorkItemID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if work.State != core.WorkDraft && work.State != core.WorkReady {
		writeError(w, http.StatusConflict, "task contract revisions are only accepted while work is DRAFT or READY")
		return
	}
	req.Material.TaskType = req.Contract.TaskType
	readiness := material.Evaluate(req.Material)
	if readiness.Status == material.Blocked {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":     "material blocked",
			"readiness": readiness,
		})
		return
	}
	route, err := routing.Resolve(req.Contract.TaskType, req.Subsystem)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	req.Contract.CapabilityIDs = route.CapabilityIDs
	req.Contract.SkillIDs = route.SkillIDs
	req.Contract.Repository = req.Material.Repository
	req.Contract.BaseCommit = req.Material.BaseCommit
	req.Contract.TargetID = req.Material.TargetID
	req.Contract.AcceptanceCriteria = append([]string(nil), req.Material.AcceptanceCriteria...)
	if !verification.ValidatePlan(req.VerificationPlan, req.Contract.AcceptanceCriteria) {
		writeError(w, http.StatusUnprocessableEntity, "verification plan must cover every acceptance criterion with at least one evidence requirement")
		return
	}
	planDigest, err := req.VerificationPlan.Digest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Contract.VerificationPlanID = req.VerificationPlan.ID
	req.Contract.VerificationPlanDigest = planDigest
	if req.Contract.Revision == 0 {
		req.Contract.Revision = 1
	}
	digest, err := req.Contract.Digest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	expectedWorkVersion := work.Version
	work.ActiveTaskContractDigest = digest
	work.ActiveRunID = ""
	if work.State == core.WorkDraft {
		if err := work.Transition(core.WorkReady); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := s.store.CreateTaskAndUpdateWork(req.Contract, req.VerificationPlan, expectedWorkVersion, work); err != nil {
		if errors.Is(err, store.ErrExists) || errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"contract":  req.Contract,
		"digest":    digest,
		"readiness": readiness,
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.store.GetTask(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	digest, err := task.Digest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contract": task, "digest": digest})
}

type createRunRequest struct {
	RunID              string                `json:"run_id"`
	TaskContractDigest string                `json:"task_contract_digest"`
	AttemptID          string                `json:"attempt_id"`
	RunInput           core.RunInputManifest `json:"run_input"`
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RunID == "" || req.TaskContractDigest == "" || req.AttemptID == "" {
		writeError(w, http.StatusBadRequest, "run_id, task_contract_digest and attempt_id are required")
		return
	}
	if req.RunInput.RuntimeProfile == "" || req.RunInput.ToolProfile == "" || req.RunInput.WorkerProfile == "" || req.RunInput.PolicyProfile == "" {
		writeError(w, http.StatusBadRequest, "runtime_profile, tool_profile, worker_profile and policy_profile are required")
		return
	}
	req.RunInput.RunID = req.RunID
	req.RunInput.TaskContractDigest = req.TaskContractDigest
	inputDigest, err := req.RunInput.Digest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	task, err := s.store.GetTaskByDigest(req.TaskContractDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	work, err := s.store.GetWork(task.WorkItemID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if work.State != core.WorkReady {
		writeError(w, http.StatusConflict, "work must be READY before starting a run")
		return
	}
	if work.ActiveTaskContractDigest != req.TaskContractDigest {
		writeError(w, http.StatusConflict, "run must use the current active task contract revision")
		return
	}
	value := run.New(req.RunID, req.TaskContractDigest, inputDigest)
	attempt, err := value.StartAttempt(req.AttemptID, s.now())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	sess := session.New(req.RunID, attempt.Epoch)
	expectedWorkVersion := work.Version
	work.ActiveRunID = req.RunID
	if err := work.Transition(core.WorkExecuting); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.store.CreateExecutionAndUpdateWork(*value, attempt, *sess, req.RunInput, expectedWorkVersion, work); err != nil {
		if errors.Is(err, store.ErrExists) || errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"run":       value,
		"attempt":   attempt,
		"session":   sess,
		"run_input": req.RunInput,
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	value, sess, err := s.store.GetExecution(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	input, err := s.store.GetRunInputByDigest(value.RunInputManifestDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": value, "session": sess, "run_input": input})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	var req epochRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	err := s.mutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
		if err := value.Pause(req.ExecutionEpoch); err != nil {
			return err
		}
		return sess.Pause(req.ExecutionEpoch)
	})
	if err != nil {
		writeMutationError(w, err)
		return
	}
	s.writeExecution(w, r.PathValue("id"))
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	var req epochRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	err := s.mutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
		if err := value.Resume(req.ExecutionEpoch); err != nil {
			return err
		}
		return sess.Resume(req.ExecutionEpoch)
	})
	if err != nil {
		writeMutationError(w, err)
		return
	}
	s.writeExecution(w, r.PathValue("id"))
}

type steerRequest struct {
	ID             string `json:"steering_command_id"`
	ExecutionEpoch uint64 `json:"execution_epoch"`
	Sequence       uint64 `json:"sequence"`
	Actor          string `json:"actor"`
	ContentDigest  string `json:"content_digest"`
}

func (s *Server) handleSteer(w http.ResponseWriter, r *http.Request) {
	var req steerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" || req.ExecutionEpoch == 0 || req.Sequence == 0 || req.Actor == "" || req.ContentDigest == "" {
		writeError(w, http.StatusBadRequest, "steering_command_id, execution_epoch, sequence, actor and content_digest are required")
		return
	}

	runID := r.PathValue("id")
	value, sess, err := s.store.GetExecution(runID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	expectedVersion := value.Version
	cmd := session.SteeringCommand{
		ID:             req.ID,
		RunID:          runID,
		ExecutionEpoch: req.ExecutionEpoch,
		Sequence:       req.Sequence,
		Actor:          req.Actor,
		ContentDigest:  req.ContentDigest,
		CreatedAt:      s.now(),
	}
	if err := value.CheckEpoch(req.ExecutionEpoch); err != nil {
		writeMutationError(w, err)
		return
	}
	if err := sess.ApplySteering(cmd); err != nil {
		writeMutationError(w, err)
		return
	}
	if err := s.store.RecordSteering(runID, expectedVersion, value, sess, cmd); err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, cmd)
}

func (s *Server) handleGetSteering(w http.ResponseWriter, r *http.Request) {
	cmd, err := s.store.GetSteering(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cmd)
}

func (s *Server) handleTakeover(w http.ResponseWriter, r *http.Request) {
	var req epochRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var newEpoch uint64
	err := s.mutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
		epoch, err := value.Takeover(req.ExecutionEpoch)
		if err != nil {
			return err
		}
		sessionEpoch, err := sess.Takeover(req.ExecutionEpoch)
		if err != nil {
			return err
		}
		if epoch != sessionEpoch {
			return errors.New("run/session epoch divergence")
		}
		newEpoch = epoch
		return nil
	})
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"control_owner":   "HUMAN",
		"execution_epoch": newEpoch,
	})
}

type createCheckpointRequest struct {
	ID                      string   `json:"checkpoint_id"`
	ExecutionEpoch          uint64   `json:"execution_epoch"`
	SourceTreeDigest        string   `json:"source_tree_digest"`
	DiffDigest              string   `json:"diff_digest,omitempty"`
	Objective               string   `json:"objective,omitempty"`
	Completed               []string `json:"completed,omitempty"`
	Pending                 []string `json:"pending,omitempty"`
	Questions               []string `json:"questions,omitempty"`
	LastEventSequence       uint64   `json:"last_event_sequence"`
	ExternalOperationCursor string   `json:"external_operation_cursor,omitempty"`
}

func (s *Server) handleCreateCheckpoint(w http.ResponseWriter, r *http.Request) {
	var req createCheckpointRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" || req.ExecutionEpoch == 0 || req.SourceTreeDigest == "" {
		writeError(w, http.StatusBadRequest, "checkpoint_id, execution_epoch and source_tree_digest are required")
		return
	}
	runID := r.PathValue("id")
	value, sess, err := s.store.GetExecution(runID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := value.CheckEpoch(req.ExecutionEpoch); err != nil {
		writeMutationError(w, err)
		return
	}
	if sess.ExecutionEpoch != req.ExecutionEpoch {
		writeError(w, http.StatusConflict, "session execution epoch does not match run")
		return
	}
	item := session.Checkpoint{
		ID:                      req.ID,
		RunID:                   runID,
		TaskContractDigest:      value.TaskContractDigest,
		RunInputManifestDigest:  value.RunInputManifestDigest,
		ExecutionEpoch:          req.ExecutionEpoch,
		SourceTreeDigest:        req.SourceTreeDigest,
		DiffDigest:              req.DiffDigest,
		Objective:               req.Objective,
		Completed:               append([]string(nil), req.Completed...),
		Pending:                 append([]string(nil), req.Pending...),
		Questions:               append([]string(nil), req.Questions...),
		LastEventSequence:       req.LastEventSequence,
		ExternalOperationCursor: req.ExternalOperationCursor,
		CreatedAt:               s.now(),
	}
	digest, err := s.store.CreateCheckpoint(item)
	if err != nil {
		if errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"checkpoint": item,
		"digest":     digest,
	})
}

func (s *Server) handleGetCheckpoint(w http.ResponseWriter, r *http.Request) {
	item, digest, err := s.store.GetCheckpoint(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"checkpoint": item,
		"digest":     digest,
	})
}

func (s *Server) handleCompleteRun(w http.ResponseWriter, r *http.Request) {
	var req epochRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	runID := r.PathValue("id")
	value, sess, err := s.store.GetExecution(runID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	expectedRunVersion := value.Version
	if err := value.Complete(req.ExecutionEpoch); err != nil {
		writeMutationError(w, err)
		return
	}
	task, err := s.store.GetTaskByDigest(value.TaskContractDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	work, err := s.store.GetWork(task.WorkItemID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if work.State != core.WorkExecuting ||
		work.ActiveTaskContractDigest != value.TaskContractDigest ||
		work.ActiveRunID != runID {
		writeError(w, http.StatusConflict, "work is not executing this run/task subject")
		return
	}
	expectedWorkVersion := work.Version
	if err := work.Transition(core.WorkVerifying); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.store.UpdateExecutionAndWork(
		runID,
		expectedRunVersion,
		value,
		sess,
		expectedWorkVersion,
		work,
	); err != nil {
		writeMutationError(w, err)
		return
	}
	s.writeExecution(w, runID)
}

type createActionRequest struct {
	ID               string           `json:"action_request_id"`
	ExecutionEpoch   uint64           `json:"execution_epoch"`
	RecoveryEpoch    *uint64          `json:"recovery_epoch"`
	Action           string           `json:"action"`
	RiskClass        action.RiskClass `json:"risk_class"`
	Capability       string           `json:"capability"`
	ParametersDigest string           `json:"parameters_digest"`
	IdempotencyKey   string           `json:"idempotency_key"`
	RequestedBy      string           `json:"requested_by"`
}

func (s *Server) handleCreateAction(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action gateway provider is not configured")
		return
	}
	var req createActionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" ||
		req.ExecutionEpoch == 0 ||
		req.RecoveryEpoch == nil ||
		req.Action == "" ||
		req.RiskClass == "" ||
		req.Capability == "" ||
		req.ParametersDigest == "" ||
		req.IdempotencyKey == "" ||
		req.RequestedBy == "" {
		writeError(w, http.StatusBadRequest, "action_request_id, execution_epoch, recovery_epoch, action, risk_class, capability, parameters_digest, idempotency_key and requested_by are required")
		return
	}

	receipt, err := s.actions.Execute(r.Context(), action.Request{
		ID:               req.ID,
		RunID:            r.PathValue("id"),
		ExecutionEpoch:   req.ExecutionEpoch,
		RecoveryEpoch:    *req.RecoveryEpoch,
		Action:           req.Action,
		RiskClass:        req.RiskClass,
		Capability:       req.Capability,
		ParametersDigest: req.ParametersDigest,
		IdempotencyKey:   req.IdempotencyKey,
		RequestedBy:      req.RequestedBy,
		RequestedAt:      s.now(),
	})
	if err != nil {
		writeActionError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

func (s *Server) handleGetAction(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action gateway provider is not configured")
		return
	}
	item, err := s.actions.Get(r.PathValue("id"))
	if err != nil {
		writeActionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleReconcileAction(w http.ResponseWriter, r *http.Request) {
	if s.actions == nil {
		writeError(w, http.StatusServiceUnavailable, "action gateway provider is not configured")
		return
	}
	receipt, err := s.actions.Reconcile(r.Context(), r.PathValue("id"))
	if err != nil {
		writeActionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func writeActionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, action.ErrDenied):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, action.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, action.ErrOperationAbsent), errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, run.ErrStaleEpoch), errors.Is(err, recovery.ErrStaleEpoch), errors.Is(err, recovery.ErrRecoveryMode):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
}

type createDeliveryRequest struct {
	ID           string             `json:"delivery_receipt_id"`
	RunID        string             `json:"run_id"`
	ResultCommit string             `json:"result_commit,omitempty"`
	Artifacts    []core.ArtifactRef `json:"artifacts,omitempty"`
	KnownLimits  []string           `json:"known_limits,omitempty"`
}

func (s *Server) handleCreateDelivery(w http.ResponseWriter, r *http.Request) {
	var req createDeliveryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" || req.RunID == "" {
		writeError(w, http.StatusBadRequest, "delivery_receipt_id and run_id are required")
		return
	}
	value, _, err := s.store.GetExecution(req.RunID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if value.State != run.Completed {
		writeError(w, http.StatusConflict, "run must be COMPLETED before creating delivery")
		return
	}
	task, err := s.store.GetTaskByDigest(value.TaskContractDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if req.ResultCommit == "" && len(req.Artifacts) == 0 {
		writeError(w, http.StatusBadRequest, "delivery requires result_commit or at least one artifact")
		return
	}
	for _, artifact := range req.Artifacts {
		if artifact.ID == "" || artifact.Digest == "" {
			writeError(w, http.StatusBadRequest, "artifact_id and digest are required")
			return
		}
	}
	item := core.DeliveryReceipt{
		ID:                 req.ID,
		WorkItemID:         task.WorkItemID,
		TaskContractDigest: value.TaskContractDigest,
		RunID:              value.ID,
		TargetID:           task.TargetID,
		BaseCommit:         task.BaseCommit,
		ResultCommit:       req.ResultCommit,
		Artifacts:          append([]core.ArtifactRef(nil), req.Artifacts...),
		KnownLimits:        append([]string(nil), req.KnownLimits...),
		CreatedAt:          s.now(),
	}
	item.SubjectDigest, err = item.CalculateSubjectDigest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.CreateDelivery(item); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleGetDelivery(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetDelivery(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type createEvidenceRequest struct {
	DeliveryReceiptID string           `json:"delivery_receipt_id"`
	Evidence          core.EvidenceRef `json:"evidence"`
}

func (s *Server) handleCreateEvidence(w http.ResponseWriter, r *http.Request) {
	var req createEvidenceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DeliveryReceiptID == "" || req.Evidence.ID == "" || req.Evidence.RequirementID == "" || req.Evidence.Issuer == "" || req.Evidence.Procedure == "" || req.Evidence.Result == "" {
		writeError(w, http.StatusBadRequest, "delivery_receipt_id, evidence_id, requirement_id, issuer, procedure and result are required")
		return
	}
	delivery, err := s.store.GetDelivery(req.DeliveryReceiptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	task, err := s.store.GetTaskByDigest(delivery.TaskContractDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	plan, err := s.store.GetVerificationPlanByDigest(task.VerificationPlanDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	requirement, ok := verification.FindRequirement(plan, req.Evidence.RequirementID)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "evidence requirement_id is not present in the frozen verification plan")
		return
	}
	if req.Evidence.Procedure != requirement.Procedure || (requirement.Issuer != "" && req.Evidence.Issuer != requirement.Issuer) {
		writeError(w, http.StatusUnprocessableEntity, "evidence issuer/procedure does not match the frozen verification requirement")
		return
	}
	item := req.Evidence
	item.DeliveryReceiptID = delivery.ID
	item.SubjectDigest = delivery.SubjectDigest
	if !verification.EvidenceArtifactsBelongToDelivery(delivery, item) {
		writeError(w, http.StatusUnprocessableEntity, "evidence artifact_refs must be unique artifacts from the exact delivery")
		return
	}
	if err := s.store.CreateEvidence(item); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetEvidence(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type createVerificationRequest struct {
	ReportID          string   `json:"verification_report_id"`
	DeliveryReceiptID string   `json:"delivery_receipt_id"`
	Verifier          string   `json:"verifier"`
	EvidenceIDs       []string `json:"evidence_ids"`
}

func (s *Server) handleCreateVerification(w http.ResponseWriter, r *http.Request) {
	var req createVerificationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ReportID == "" || req.DeliveryReceiptID == "" || req.Verifier == "" {
		writeError(w, http.StatusBadRequest, "verification_report_id, delivery_receipt_id and verifier are required")
		return
	}
	delivery, err := s.store.GetDelivery(req.DeliveryReceiptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	task, err := s.store.GetTaskByDigest(delivery.TaskContractDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	plan, err := s.store.GetVerificationPlanByDigest(task.VerificationPlanDigest)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	evidence := make([]core.EvidenceRef, 0, len(req.EvidenceIDs))
	for _, id := range req.EvidenceIDs {
		item, err := s.store.GetEvidence(id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		evidence = append(evidence, item)
	}
	report := verification.Evaluate(plan, delivery.SubjectDigest, evidence)
	report.ID = req.ReportID
	report.VerificationPlanDigest = task.VerificationPlanDigest
	report.DeliveryReceiptID = delivery.ID
	report.Verifier = req.Verifier
	report.CreatedAt = s.now()
	if err := s.store.CreateVerification(report); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusCreated
	if report.Result != "PASS" {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, report)
}

func (s *Server) handleGetVerification(w http.ResponseWriter, r *http.Request) {
	report, err := s.store.GetVerification(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type createReviewRequest struct {
	ReportID             string           `json:"review_report_id"`
	DeliveryReceiptID    string           `json:"delivery_receipt_id"`
	VerificationReportID string           `json:"verification_report_id"`
	Reviewer             string           `json:"reviewer"`
	Result               string           `json:"result"`
	Findings             []review.Finding `json:"findings,omitempty"`
	KnownLimits          []string         `json:"known_limits,omitempty"`
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	var req createReviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ReportID == "" || req.DeliveryReceiptID == "" || req.VerificationReportID == "" || req.Reviewer == "" || req.Result == "" {
		writeError(w, http.StatusBadRequest, "review_report_id, delivery_receipt_id, verification_report_id, reviewer and result are required")
		return
	}
	delivery, err := s.store.GetDelivery(req.DeliveryReceiptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	verificationReport, err := s.store.GetVerification(req.VerificationReportID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if verificationReport.Result != "PASS" ||
		verificationReport.DeliveryReceiptID != delivery.ID ||
		verificationReport.SubjectDigest != delivery.SubjectDigest {
		writeError(w, http.StatusUnprocessableEntity, "review requires PASS verification for the exact delivery subject")
		return
	}
	work, err := s.store.GetWork(delivery.WorkItemID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if work.State != core.WorkVerifying ||
		work.ActiveTaskContractDigest != delivery.TaskContractDigest ||
		work.ActiveRunID != delivery.RunID {
		writeError(w, http.StatusConflict, "work is not verifying this delivery subject")
		return
	}
	if req.Reviewer == verificationReport.Verifier || req.Reviewer == work.HumanOwner {
		writeError(w, http.StatusUnprocessableEntity, "reviewer must be independent from verifier and work owner")
		return
	}
	report := review.Report{
		ID:                   req.ReportID,
		DeliveryReceiptID:    delivery.ID,
		VerificationReportID: verificationReport.ID,
		TaskContractDigest:   delivery.TaskContractDigest,
		SubjectDigest:        delivery.SubjectDigest,
		Reviewer:             req.Reviewer,
		Result:               req.Result,
		Findings:             append([]review.Finding(nil), req.Findings...),
		KnownLimits:          append([]string(nil), req.KnownLimits...),
		CreatedAt:            s.now(),
	}
	if err := report.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	expectedVersion := work.Version
	nextWork := work
	if report.Result == review.ResultPass {
		if err := nextWork.Transition(core.WorkReviewing); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := s.store.CreateReviewAndUpdateWork(report, expectedVersion, nextWork); err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, report)
}

func (s *Server) handleGetReview(w http.ResponseWriter, r *http.Request) {
	report, err := s.store.GetReview(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type createClosureRequest struct {
	ID                   string `json:"closure_receipt_id"`
	DeliveryReceiptID    string `json:"delivery_receipt_id"`
	VerificationReportID string `json:"verification_report_id"`
	ReviewReportID       string `json:"review_report_id"`
}

func (s *Server) handleCreateClosure(w http.ResponseWriter, r *http.Request) {
	var req createClosureRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" || req.DeliveryReceiptID == "" || req.VerificationReportID == "" || req.ReviewReportID == "" {
		writeError(w, http.StatusBadRequest, "closure_receipt_id, delivery_receipt_id, verification_report_id and review_report_id are required")
		return
	}
	delivery, err := s.store.GetDelivery(req.DeliveryReceiptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	report, err := s.store.GetVerification(req.VerificationReportID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if report.Result != "PASS" || report.DeliveryReceiptID != delivery.ID || report.SubjectDigest != delivery.SubjectDigest {
		writeError(w, http.StatusUnprocessableEntity, "closure requires PASS verification for the exact delivery subject")
		return
	}
	reviewReport, err := s.store.GetReview(req.ReviewReportID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if reviewReport.Result != review.ResultPass ||
		reviewReport.DeliveryReceiptID != delivery.ID ||
		reviewReport.VerificationReportID != report.ID ||
		reviewReport.TaskContractDigest != delivery.TaskContractDigest ||
		reviewReport.SubjectDigest != delivery.SubjectDigest {
		writeError(w, http.StatusUnprocessableEntity, "closure requires PASS independent review for the exact verified delivery subject")
		return
	}
	value, _, err := s.store.GetExecution(delivery.RunID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if value.State != run.Completed || value.TaskContractDigest != delivery.TaskContractDigest {
		writeError(w, http.StatusConflict, "delivery does not match a completed run")
		return
	}
	work, err := s.store.GetWork(delivery.WorkItemID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if work.State != core.WorkReviewing || work.ActiveRunID != delivery.RunID || work.ActiveTaskContractDigest != delivery.TaskContractDigest {
		writeError(w, http.StatusConflict, "work is not reviewing this delivery subject")
		return
	}
	expectedVersion := work.Version
	if err := work.Transition(core.WorkClosed); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	closure := core.ClosureReceipt{
		ID:                   req.ID,
		WorkItemID:           delivery.WorkItemID,
		TaskContractDigest:   delivery.TaskContractDigest,
		RunID:                delivery.RunID,
		DeliveryReceiptID:    delivery.ID,
		VerificationReportID: report.ID,
		ReviewReportID:       req.ReviewReportID,
		SubjectDigest:        delivery.SubjectDigest,
		Result:               "CLOSED",
		CreatedAt:            s.now(),
	}
	if err := s.store.CreateClosureAndUpdateWork(closure, expectedVersion, work); err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, closure)
}

func (s *Server) handleGetClosure(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetClosure(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) mutateExecution(id string, fn func(*run.Run, *session.Session) error) error {
	value, sess, err := s.store.GetExecution(id)
	if err != nil {
		return err
	}
	expectedVersion := value.Version
	if err := fn(&value, &sess); err != nil {
		return err
	}
	return s.store.UpdateExecution(id, expectedVersion, value, sess)
}

func (s *Server) writeExecution(w http.ResponseWriter, id string) {
	value, sess, err := s.store.GetExecution(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": value, "session": sess})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func writeMutationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, store.ErrExists) || errors.Is(err, store.ErrConflict) || errors.Is(err, run.ErrStaleEpoch) || errors.Is(err, session.ErrStaleEpoch) || errors.Is(err, session.ErrSequence) || errors.Is(err, session.ErrRuntimeNotOwner) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusUnprocessableEntity, err.Error())
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
