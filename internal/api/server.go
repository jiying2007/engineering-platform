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
	"github.com/jiying2007/engineering-platform/internal/store"
)

type Server struct {
	store *store.Memory
	mux   *http.ServeMux
	now   func() time.Time
}

func NewServer(memory *store.Memory) *Server {
	if memory == nil {
		memory = store.NewMemory()
	}
	s := &Server{
		store: memory,
		mux:   http.NewServeMux(),
		now:   func() time.Time { return time.Now().UTC() },
	}
	s.routes()
	return s
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
	if err := s.store.CreateRun(*value); err != nil {
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
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.GetRun(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
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
