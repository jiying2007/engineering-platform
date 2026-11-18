package local

import (
	"errors"
	"time"
)

const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateWaiting   = "waiting"
	StateDone      = "done"
	StateFailed    = "failed"
	StateCancelled = "cancelled"

	StagePrepare = "prepare"
	StageExecute = "execute"
	StageVerify  = "verify"
	StageDeliver = "deliver"
)

var (
	ErrNotFound          = errors.New("record not found")
	ErrInvalidState      = errors.New("operation is not valid in the current task state")
	ErrConflict          = errors.New("record changed or immutable input does not match")
	ErrProcessUnresolved = errors.New("previous local process has not been confirmed stopped")
)

type Check struct {
	Name    string   `json:"name" yaml:"name"`
	Argv    []string `json:"argv" yaml:"argv"`
	Timeout string   `json:"timeout" yaml:"timeout"`
}

// Project is local registration plus a small, frozen project configuration.
// The registration name is supplied by the CLI, not by executable project code.
type Project struct {
	Name      string   `json:"name" yaml:"-"`
	Root      string   `json:"root" yaml:"root"`
	Base      string   `json:"base" yaml:"base"`
	Runtime   string   `json:"runtime" yaml:"runtime"`
	Checks    []Check  `json:"checks" yaml:"checks"`
	Artifacts []string `json:"artifacts" yaml:"artifacts"`
}

// Runtime never contains environment variables, bearer tokens or credentials.
// Its argv is explicit: no shell interpolation or implicit model selection.
type Runtime struct {
	Name    string   `json:"name" yaml:"-"`
	Argv    []string `json:"argv" yaml:"argv"`
	Stdin   string   `json:"stdin" yaml:"stdin"`
	Timeout string   `json:"timeout" yaml:"timeout"`
	Model   bool     `json:"model" yaml:"model"`
}

type Snapshot struct {
	Project Project `json:"project"`
	Runtime Runtime `json:"runtime"`
}

type Task struct {
	ID             string    `json:"id"`
	Project        string    `json:"project"`
	Owner          string    `json:"owner"`
	Instruction    string    `json:"instruction"`
	Config         Snapshot  `json:"config"`
	ConfigDigest   string    `json:"config_digest"`
	BaseSHA        string    `json:"base_sha,omitempty"`
	Branch         string    `json:"branch,omitempty"`
	Worktree       string    `json:"worktree,omitempty"`
	State          string    `json:"state"`
	Stage          string    `json:"stage"`
	Reason         string    `json:"reason,omitempty"`
	CurrentAttempt int       `json:"current_attempt"`
	StopRequested  bool      `json:"stop_requested"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ProcessIdentity struct {
	PID        int    `json:"pid"`
	PGID       int    `json:"pgid"`
	StartTicks uint64 `json:"start_ticks"`
	BootID     string `json:"boot_id"`
}

type CheckResult struct {
	Name         string    `json:"name"`
	SourceCommit string    `json:"source_commit"`
	Argv         []string  `json:"argv"`
	ExitCode     int       `json:"exit_code"`
	Status       string    `json:"status"`
	Stdout       string    `json:"stdout"`
	Stderr       string    `json:"stderr"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
}

type Artifact struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type Usage struct {
	Known bool   `json:"known"`
	Note  string `json:"note"`
}

// Attempt inputs are immutable after claim. A resolved starting commit can be
// filled once during preparation, before a runtime has been started.
type Attempt struct {
	ID             string           `json:"id"`
	TaskID         string           `json:"task_id"`
	Number         int              `json:"number"`
	Input          string           `json:"input"`
	StartCommit    string           `json:"start_commit,omitempty"`
	Config         Snapshot         `json:"config"`
	ConfigDigest   string           `json:"config_digest"`
	RuntimeVersion string           `json:"runtime_version,omitempty"`
	State          string           `json:"state"`
	Stage          string           `json:"stage"`
	Reason         string           `json:"reason,omitempty"`
	Process        *ProcessIdentity `json:"process,omitempty"`
	RuntimeStarted bool             `json:"runtime_started"`
	Result         RunResult        `json:"result"`
	StartedAt      time.Time        `json:"started_at"`
	EndedAt        time.Time        `json:"ended_at,omitempty"`
}

// Events are an audit trail of important actions, not a replay engine.
type Event struct {
	ID        int64          `json:"id"`
	TaskID    string         `json:"task_id"`
	AttemptID string         `json:"attempt_id,omitempty"`
	Kind      string         `json:"kind"`
	Actor     string         `json:"actor"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

func validState(state string) bool {
	switch state {
	case StateQueued, StateRunning, StateWaiting, StateDone, StateFailed, StateCancelled:
		return true
	}
	return false
}

func validStage(stage string) bool {
	switch stage {
	case StagePrepare, StageExecute, StageVerify, StageDeliver:
		return true
	}
	return false
}

func terminalAttemptState(state string) bool {
	return state == StateWaiting || state == StateDone || state == StateFailed || state == StateCancelled
}
