package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/routing"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
	now   func() time.Time
}

func NewServer(s store.Store) *Server {
	if s == nil {
		s = store.NewMemory()
	}
	server := &Server{
		store: s,
		mux:   http.NewServeMux(),
		now:   func() time.Time { return time.Now().UTC() },
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
	s.mux.HandleFunc("POST /api/v1/work-items", s.handleCreateWork)
	s.mux.HandleFunc("GET /api/v1/work-items/{id}", s.handleGetWork)
	s.mux.HandleFunc("POST /api/v1/task-contracts", s.handleCreateTask)
	s.mux.HandleFunc("GET /api/v1/task-contracts/{id}", s.handleGetTask)
	s.mux.HandleFunc("POST /api/v1/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /api/v1/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/steer", s.handleSteer)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/pause", s.handlePause)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/resume", s.handleResume)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/takeover", s.handleTakeover)
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
	Contract  core.TaskContract `json:"contract"`
	Material  material.Manifest `json:"material"`
	Subsystem string            `json:"subsystem,omitempty"`
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
	if _, err := s.store.GetWork(req.Contract.WorkItemID); err != nil {
		writeStoreError(w, err)
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
	if req.Contract.Revision == 0 {
		req.Contract.Revision = 1
	}
	digest, err := req.Contract.Digest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.CreateTask(req.Contract); err != nil {
		if errors.Is(err, store.ErrExists) {
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
	RunID          string `json:"run_id"`
	TaskContractID string `json:"task_contract_id"`
	AttemptID      string `json:"attempt_id"`
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RunID == "" || req.TaskContractID == "" || req.AttemptID == "" {
		writeError(w, http.StatusBadRequest, "run_id, task_contract_id and attempt_id are required")
		return
	}
	task, err := s.store.GetTask(req.TaskContractID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	digest, err := task.Digest()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	value := run.New(req.RunID, digest)
	attempt, err := value.StartAttempt(req.AttemptID, s.now())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	sess := session.New(req.RunID, attempt.Epoch)
	if err := s.store.CreateExecution(*value, *sess); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"run":     value,
		"attempt": attempt,
		"session": sess,
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	value, sess, err := s.store.GetExecution(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": value, "session": sess})
}

type epochRequest struct {
	ExecutionEpoch uint64 `json:"execution_epoch"`
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	var req epochRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	err := s.store.MutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
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
	err := s.store.MutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
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
	cmd := session.SteeringCommand{
		ID:             req.ID,
		RunID:          r.PathValue("id"),
		ExecutionEpoch: req.ExecutionEpoch,
		Sequence:       req.Sequence,
		Actor:          req.Actor,
		ContentDigest:  req.ContentDigest,
		CreatedAt:      s.now(),
	}
	err := s.store.MutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
		if err := value.CheckEpoch(req.ExecutionEpoch); err != nil {
			return err
		}
		return sess.ApplySteering(cmd)
	})
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, cmd)
}

func (s *Server) handleTakeover(w http.ResponseWriter, r *http.Request) {
	var req epochRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var newEpoch uint64
	err := s.store.MutateExecution(r.PathValue("id"), func(value *run.Run, sess *session.Session) error {
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
	if errors.Is(err, run.ErrStaleEpoch) || errors.Is(err, session.ErrStaleEpoch) || errors.Is(err, session.ErrSequence) || errors.Is(err, session.ErrRuntimeNotOwner) {
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
