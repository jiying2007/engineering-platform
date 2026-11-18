package local

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const schemaVersion = 1
const sqliteApplicationID = 0x454e4731 // ENG1: never adopt an unrelated SQLite file.
const storedTime = "2006-01-02T15:04:05.000000000Z"

type Store struct {
	db      *sql.DB
	dataDir string
}

func OpenStore(dataDir string) (*Store, error) {
	path, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("data directory must be a real, owner-private directory (0700)")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(path, "eng.db")
	if info, err := os.Lstat(dbPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database path must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: dbPath}
	dsn := u.String() + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_synchronous=FULL&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open task database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, dataDir: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(dbPath, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("protect task database: %w", err)
	}
	return store, nil
}

func (store *Store) DataDir() string { return store.dataDir }
func (store *Store) Close() error    { return store.db.Close() }

// DB is exposed for the in-process consistent backup operation only.
// Normal task mutations go through Store so their event is committed together.
func (store *Store) DB() *sql.DB { return store.db }

func (store *Store) initialize(ctx context.Context) error {
	var mode string
	if err := store.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("configure task database journal: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("task database requires local WAL support")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("open schema transaction: %w", err)
	}
	defer tx.Rollback()
	var version, applicationID int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil {
		return err
	}
	if version == schemaVersion && applicationID == sqliteApplicationID {
		return tx.Commit()
	}
	if version != 0 || applicationID != 0 {
		return fmt.Errorf("unsupported task database identity/schema %d; use its matching eng version or an explicit migration", version)
	}
	var tables int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return err
	}
	if tables != 0 {
		return fmt.Errorf("refusing to initialize an unrelated nonempty SQLite database")
	}
	const schema = `
CREATE TABLE tasks (
 id TEXT PRIMARY KEY, project TEXT NOT NULL, owner TEXT NOT NULL,
 instruction TEXT NOT NULL, config_json TEXT NOT NULL, config_digest TEXT NOT NULL,
 base_sha TEXT NOT NULL DEFAULT '', branch TEXT NOT NULL DEFAULT '', worktree TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL CHECK(state IN ('queued','running','waiting','done','failed','cancelled')),
 stage TEXT NOT NULL CHECK(stage IN ('prepare','execute','verify','deliver')),
 reason TEXT NOT NULL DEFAULT '', current_attempt INTEGER NOT NULL DEFAULT 0 CHECK(current_attempt >= 0),
 stop_requested INTEGER NOT NULL DEFAULT 0 CHECK(stop_requested IN (0,1)),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE attempts (
 id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id),
 number INTEGER NOT NULL CHECK(number > 0), input TEXT NOT NULL, start_commit TEXT NOT NULL DEFAULT '',
 config_json TEXT NOT NULL, config_digest TEXT NOT NULL, runtime_version TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL CHECK(state IN ('running','waiting','done','failed','cancelled')),
 stage TEXT NOT NULL CHECK(stage IN ('prepare','execute','verify','deliver')),
 reason TEXT NOT NULL DEFAULT '', process_json TEXT NOT NULL DEFAULT 'null',
 runtime_started INTEGER NOT NULL DEFAULT 0 CHECK(runtime_started IN (0,1)),
 result_json TEXT NOT NULL DEFAULT '{}', started_at TEXT NOT NULL, ended_at TEXT NOT NULL DEFAULT '',
 UNIQUE(task_id, number)
);
CREATE TABLE events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT NOT NULL REFERENCES tasks(id),
 attempt_id TEXT REFERENCES attempts(id), kind TEXT NOT NULL, actor TEXT NOT NULL,
 data_json TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX tasks_by_state ON tasks(state, created_at);
CREATE INDEX events_by_task ON events(task_id, id);
`
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create task schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=%d", sqliteApplicationID, schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) CreateTask(ctx context.Context, task Task) (Task, error) {
	if strings.TrimSpace(task.Instruction) == "" || len(task.Instruction) > maxConfigBytes {
		return Task{}, fmt.Errorf("task instruction must be nonempty and at most 1 MiB")
	}
	config, digest, err := Freeze(task.Config.Project, task.Config.Runtime)
	if err != nil {
		return Task{}, err
	}
	if task.Project == "" {
		task.Project = config.Project.Name
	}
	if task.Project != config.Project.Name || (task.ConfigDigest != "" && task.ConfigDigest != digest) {
		return Task{}, ErrConflict
	}
	if task.ID == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return Task{}, err
		}
		task.ID = "task-" + hex.EncodeToString(random[:])
	}
	if !configName.MatchString(task.ID) || strings.ContainsRune(task.Owner, '\x00') || len(task.Owner) > 256 {
		return Task{}, fmt.Errorf("invalid task identity or owner")
	}
	if task.Owner == "" {
		task.Owner = "local"
	}
	task.Config, task.ConfigDigest = config, digest
	task.State, task.Stage, task.Reason = StateQueued, StagePrepare, ""
	task.CurrentAttempt, task.StopRequested = 0, false
	task.BaseSHA, task.Branch, task.Worktree = "", "", ""
	task.CreatedAt = time.Now().UTC()
	task.UpdatedAt = task.CreatedAt
	configJSON, err := encodeJSON(task.Config)
	if err != nil {
		return Task{}, err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks
 (id,project,owner,instruction,config_json,config_digest,state,stage,created_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?)`, task.ID, task.Project, task.Owner, task.Instruction, configJSON, digest, task.State, task.Stage, formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, Kind: "task.submitted", Actor: task.Owner, Data: map[string]any{"project": task.Project, "config_digest": digest}}); err != nil {
		return Task{}, err
	}
	return task, tx.Commit()
}

const taskColumns = `id,project,owner,instruction,config_json,config_digest,base_sha,branch,worktree,state,stage,reason,current_attempt,stop_requested,created_at,updated_at`
const attemptColumns = `id,task_id,number,input,start_commit,config_json,config_digest,runtime_version,state,stage,reason,process_json,runtime_started,result_json,started_at,ended_at`

type rowScanner interface{ Scan(...any) error }

func scanTask(row rowScanner) (Task, error) {
	var task Task
	var config, created, updated string
	err := row.Scan(&task.ID, &task.Project, &task.Owner, &task.Instruction, &config, &task.ConfigDigest, &task.BaseSHA, &task.Branch, &task.Worktree, &task.State, &task.Stage, &task.Reason, &task.CurrentAttempt, &task.StopRequested, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	if err := json.Unmarshal([]byte(config), &task.Config); err != nil {
		return Task{}, fmt.Errorf("invalid stored task configuration")
	}
	if task.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Task{}, fmt.Errorf("invalid stored task timestamp")
	}
	if task.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Task{}, fmt.Errorf("invalid stored task timestamp")
	}
	return task, nil
}

func scanAttempt(row rowScanner) (Attempt, error) {
	var attempt Attempt
	var config, process, result, started, ended string
	err := row.Scan(&attempt.ID, &attempt.TaskID, &attempt.Number, &attempt.Input, &attempt.StartCommit, &config, &attempt.ConfigDigest, &attempt.RuntimeVersion, &attempt.State, &attempt.Stage, &attempt.Reason, &process, &attempt.RuntimeStarted, &result, &started, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, ErrNotFound
	}
	if err != nil {
		return Attempt{}, err
	}
	if json.Unmarshal([]byte(config), &attempt.Config) != nil || json.Unmarshal([]byte(process), &attempt.Process) != nil || json.Unmarshal([]byte(result), &attempt.Result) != nil {
		return Attempt{}, fmt.Errorf("invalid stored attempt data")
	}
	if attempt.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
		return Attempt{}, fmt.Errorf("invalid stored attempt timestamp")
	}
	if ended != "" {
		if attempt.EndedAt, err = time.Parse(time.RFC3339Nano, ended); err != nil {
			return Attempt{}, fmt.Errorf("invalid stored attempt timestamp")
		}
	}
	return attempt, nil
}

func (store *Store) GetTask(ctx context.Context, id string) (Task, error) {
	return scanTask(store.db.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", id))
}

func (store *Store) GetAttempt(ctx context.Context, id string) (Attempt, error) {
	return scanAttempt(store.db.QueryRowContext(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE id=?", id))
}

func (store *Store) ListTasks(ctx context.Context) ([]Task, error) {
	return store.listTasks(ctx, "ORDER BY created_at DESC,id")
}

func (store *Store) ListUnfinished(ctx context.Context) ([]Task, error) {
	return store.listTasks(ctx, "WHERE state IN ('queued','running','waiting') ORDER BY created_at,id")
}

func (store *Store) listTasks(ctx context.Context, suffix string) ([]Task, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks "+suffix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (store *Store) ListAttempts(ctx context.Context, taskID string) ([]Attempt, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE task_id=? ORDER BY number", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := make([]Attempt, 0)
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

// ClaimTask atomically changes queued to running and records its new attempt.
// No command, Git operation or model call is made while this transaction lives.
func (store *Store) ClaimTask(ctx context.Context, taskID string) (Task, Attempt, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, Attempt{}, err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", taskID))
	if err != nil {
		return Task{}, Attempt{}, err
	}
	if task.State != StateQueued || task.StopRequested {
		return Task{}, Attempt{}, ErrInvalidState
	}
	digest, err := snapshotDigest(task.Config)
	if err != nil || digest != task.ConfigDigest {
		return Task{}, Attempt{}, ErrConflict
	}
	now := time.Now().UTC()
	task.CurrentAttempt++
	task.State, task.Stage, task.Reason, task.UpdatedAt = StateRunning, StagePrepare, "", now
	attempt := Attempt{ID: fmt.Sprintf("%s.a%d", task.ID, task.CurrentAttempt), TaskID: task.ID, Number: task.CurrentAttempt, Input: task.Instruction, Config: task.Config, ConfigDigest: task.ConfigDigest, State: StateRunning, Stage: StagePrepare, StartedAt: now}
	attempt.Result = RunResult{Status: StateRunning, RuntimeExitCode: -1, LocalProcessStopped: true, ChecksStatus: "not_run", Checks: []CheckResult{}, Artifacts: []Artifact{}, Usage: Usage{Known: false, Note: "usage has not been observed"}}
	config, _ := encodeJSON(attempt.Config)
	result, _ := encodeJSON(attempt.Result)
	_, err = tx.ExecContext(ctx, `INSERT INTO attempts(id,task_id,number,input,config_json,config_digest,state,stage,result_json,started_at)
 VALUES(?,?,?,?,?,?,?,?,?,?)`, attempt.ID, task.ID, attempt.Number, attempt.Input, config, attempt.ConfigDigest, attempt.State, attempt.Stage, result, formatTime(now))
	if err != nil {
		return Task{}, Attempt{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET state='running',stage='prepare',reason='',current_attempt=?,updated_at=? WHERE id=? AND state='queued'`, task.CurrentAttempt, formatTime(now), task.ID); err != nil {
		return Task{}, Attempt{}, err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "attempt.claimed", Actor: task.Owner, Data: map[string]any{"number": attempt.Number, "config_digest": attempt.ConfigDigest}}); err != nil {
		return Task{}, Attempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, Attempt{}, err
	}
	return task, attempt, nil
}

// UpdateTask persists preparation/stage progress without overwriting a stop
// request from another CLI or accepting an untracked state transition.
func (store *Store) UpdateTask(ctx context.Context, task Task) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", task.ID))
	if err != nil {
		return err
	}
	if current.State != StateRunning || task.State != StateRunning || current.CurrentAttempt != task.CurrentAttempt {
		return ErrInvalidState
	}
	if !sameTaskInput(current, task) || !validStage(task.Stage) || stageRank(task.Stage) < stageRank(current.Stage) || changedNonempty(current.BaseSHA, task.BaseSHA) || changedNonempty(current.Branch, task.Branch) || changedNonempty(current.Worktree, task.Worktree) {
		return ErrConflict
	}
	if task.BaseSHA != "" && !validCommit(task.BaseSHA) {
		return fmt.Errorf("task baseline must be an exact commit")
	}
	if task.Worktree != "" && !filepath.IsAbs(task.Worktree) {
		return fmt.Errorf("worktree must be an absolute path")
	}
	_, err = tx.ExecContext(ctx, "UPDATE tasks SET base_sha=?,branch=?,worktree=?,stage=?,updated_at=? WHERE id=?", task.BaseSHA, task.Branch, task.Worktree, task.Stage, formatTime(time.Now().UTC()), task.ID)
	if err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, Kind: "task.progress", Actor: task.Owner, Data: map[string]any{"stage": task.Stage, "base_sha": task.BaseSHA, "branch": task.Branch, "worktree": task.Worktree}}); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) UpdateAttempt(ctx context.Context, attempt Attempt) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := scanAttempt(tx.QueryRowContext(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE id=?", attempt.ID))
	if err != nil {
		return err
	}
	task, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", current.TaskID))
	if err != nil {
		return err
	}
	if task.CurrentAttempt != current.Number || (task.State != StateRunning && task.State != StateWaiting) || attempt.State != current.State {
		return ErrInvalidState
	}
	if err := validateAttemptUpdate(current, attempt); err != nil {
		return err
	}
	if err := persistAttempt(ctx, tx, attempt); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "attempt.progress", Actor: task.Owner, Data: map[string]any{"stage": attempt.Stage, "runtime_started": attempt.RuntimeStarted, "process": attempt.Process, "start_commit": attempt.StartCommit}}); err != nil {
		return err
	}
	return tx.Commit()
}

// FinishAttempt atomically stores the result, task state and corresponding
// event. A caller whose execution context was cancelled must use a separately
// bounded cleanup context to retain this result.
func (store *Store) FinishAttempt(ctx context.Context, task Task, attempt Attempt) error {
	if !terminalAttemptState(task.State) || !validStage(task.Stage) || task.ID != attempt.TaskID || task.CurrentAttempt != attempt.Number {
		return ErrInvalidState
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	currentTask, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", task.ID))
	if err != nil {
		return err
	}
	current, err := scanAttempt(tx.QueryRowContext(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE id=?", attempt.ID))
	if err != nil {
		return err
	}
	if currentTask.CurrentAttempt != attempt.Number || !sameTaskInput(currentTask, task) || !sameAttemptInput(current, attempt) {
		return ErrConflict
	}
	if changedNonempty(current.StartCommit, attempt.StartCommit) || changedNonempty(current.RuntimeVersion, attempt.RuntimeVersion) || current.Process != nil && attempt.Process == nil {
		return ErrConflict
	}
	if currentTask.State != StateRunning && currentTask.State != StateWaiting {
		if currentTask.State == task.State && current.State == task.State && current.Reason == task.Reason && jsonEqual(current.Result, attempt.Result) {
			return nil
		}
		return ErrInvalidState
	}
	if attempt.Process != nil && !attempt.Result.LocalProcessStopped && task.State != StateWaiting {
		return ErrProcessUnresolved
	}
	if task.State == StateDone && (attempt.Result.RemoteOutcomeUnknown || currentTask.StopRequested) {
		return ErrInvalidState
	}
	if current.RuntimeStarted && !attempt.RuntimeStarted {
		return ErrConflict
	}
	attempt.State, attempt.Stage, attempt.Reason = task.State, task.Stage, task.Reason
	if attempt.Process == nil || attempt.Result.LocalProcessStopped {
		attempt.EndedAt = time.Now().UTC()
	} else {
		attempt.EndedAt = time.Time{}
	}
	if err := persistAttempt(ctx, tx, attempt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET state=?,stage=?,reason=?,updated_at=? WHERE id=?", task.State, task.Stage, task.Reason, formatTime(time.Now().UTC()), task.ID); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "attempt.finished", Actor: task.Owner, Data: map[string]any{"state": task.State, "stage": task.Stage, "reason": task.Reason, "local_process_stopped": attempt.Result.LocalProcessStopped, "remote_outcome_unknown": attempt.Result.RemoteOutcomeUnknown}}); err != nil {
		return err
	}
	return tx.Commit()
}

func validateAttemptUpdate(current, next Attempt) error {
	if !sameAttemptInput(current, next) || !validStage(next.Stage) || stageRank(next.Stage) < stageRank(current.Stage) || current.RuntimeStarted && !next.RuntimeStarted || !next.EndedAt.Equal(current.EndedAt) {
		return ErrConflict
	}
	if changedNonempty(current.StartCommit, next.StartCommit) || (current.StartCommit == "" && next.StartCommit != "" && (current.RuntimeStarted || next.RuntimeStarted)) {
		return ErrConflict
	}
	if next.StartCommit != "" && !validCommit(next.StartCommit) {
		return fmt.Errorf("attempt start must be an exact commit")
	}
	if changedNonempty(current.RuntimeVersion, next.RuntimeVersion) {
		return ErrConflict
	}
	if next.Process != nil && (next.Process.PID <= 0 || next.Process.PGID <= 0 || next.Process.StartTicks == 0 || next.Process.BootID == "") {
		return fmt.Errorf("process identity requires PID, group, start time and boot identity")
	}
	return nil
}

func persistAttempt(ctx context.Context, tx *sql.Tx, attempt Attempt) error {
	process, err := encodeJSON(attempt.Process)
	if err != nil {
		return err
	}
	result, err := encodeJSON(attempt.Result)
	if err != nil {
		return err
	}
	ended := ""
	if !attempt.EndedAt.IsZero() {
		ended = formatTime(attempt.EndedAt)
	}
	_, err = tx.ExecContext(ctx, `UPDATE attempts SET start_commit=?,runtime_version=?,state=?,stage=?,reason=?,process_json=?,runtime_started=?,result_json=?,ended_at=? WHERE id=?`, attempt.StartCommit, attempt.RuntimeVersion, attempt.State, attempt.Stage, attempt.Reason, process, attempt.RuntimeStarted, result, ended, attempt.ID)
	return err
}

func (store *Store) StopRequested(ctx context.Context, taskID string) (bool, error) {
	var requested bool
	err := store.db.QueryRowContext(ctx, "SELECT stop_requested FROM tasks WHERE id=?", taskID).Scan(&requested)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return requested, err
}

func (store *Store) RequestStop(ctx context.Context, taskID string) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", taskID))
	if err != nil {
		return err
	}
	if task.State == StateCancelled || task.StopRequested {
		return nil
	}
	if task.State != StateQueued && task.State != StateRunning && task.State != StateWaiting {
		return ErrInvalidState
	}
	state, reason := task.State, task.Reason
	if task.State == StateQueued {
		state, reason = StateCancelled, "cancelled_before_start"
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET stop_requested=1,state=?,reason=?,updated_at=? WHERE id=?", state, reason, formatTime(time.Now().UTC()), task.ID); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, Kind: "task.stop_requested", Actor: task.Owner, Data: map[string]any{"state": state}}); err != nil {
		return err
	}
	return tx.Commit()
}

// EnqueueResume records an explicit user decision to create a new attempt.
// It never claims or starts a runtime and cannot bypass a live/unknown local
// process. Prior attempt inputs/results remain intact.
func (store *Store) EnqueueResume(ctx context.Context, taskID, input string) (Task, error) {
	if len(input) > maxConfigBytes {
		return Task{}, fmt.Errorf("task instruction exceeds 1 MiB")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", taskID))
	if err != nil {
		return Task{}, err
	}
	if task.State != StateWaiting && task.State != StateFailed && task.State != StateCancelled {
		return Task{}, ErrInvalidState
	}
	if task.CurrentAttempt > 0 {
		previous, err := scanAttempt(tx.QueryRowContext(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE task_id=? AND number=?", task.ID, task.CurrentAttempt))
		if err != nil {
			return Task{}, err
		}
		if previous.Process != nil && !previous.Result.LocalProcessStopped {
			return Task{}, ErrProcessUnresolved
		}
	}
	if input != "" {
		if strings.TrimSpace(input) == "" {
			return Task{}, fmt.Errorf("task instruction cannot contain only whitespace")
		}
		task.Instruction = input
	}
	task.State, task.Stage, task.Reason, task.StopRequested = StateQueued, StagePrepare, "", false
	task.UpdatedAt = time.Now().UTC()
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET instruction=?,state='queued',stage='prepare',reason='',stop_requested=0,updated_at=? WHERE id=?", task.Instruction, formatTime(task.UpdatedAt), task.ID); err != nil {
		return Task{}, err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, Kind: "task.resume_requested", Actor: task.Owner, Data: map[string]any{"previous_attempt": task.CurrentAttempt, "input_updated": input != ""}}); err != nil {
		return Task{}, err
	}
	return task, tx.Commit()
}

// MarkRestored pauses every restored queued item in the same transaction as
// its audit event. Running items remain running for Recover to inspect; a
// backup can never prove that effects after its snapshot did not occur.
func (store *Store) MarkRestored(ctx context.Context) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,state,
 EXISTS (SELECT 1 FROM events WHERE task_id=task.id AND kind='task.restored_reconcile') FROM tasks AS task
 ORDER BY created_at,id`)
	if err != nil {
		return err
	}
	type restoredTask struct {
		id, state  string
		reconciled bool
	}
	var tasks []restoredTask
	for rows.Next() {
		var task restoredTask
		if err := rows.Scan(&task.id, &task.state, &task.reconciled); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, task)
	}
	rowErr := rows.Err()
	if err := rows.Close(); err != nil {
		return err
	}
	if rowErr != nil {
		return rowErr
	}
	for _, task := range tasks {
		if task.state == StateQueued {
			if _, err := tx.ExecContext(ctx, "UPDATE tasks SET state='waiting',reason='restored_queue_requires_explicit_resume',updated_at=? WHERE id=? AND state='queued'", formatTime(time.Now().UTC()), task.id); err != nil {
				return err
			}
			if err := insertEvent(ctx, tx, Event{TaskID: task.id, Kind: "task.restored_paused", Actor: "eng.restore", Data: map[string]any{"previous_state": "queued", "automatic_replay": false}}); err != nil {
				return err
			}
		}
		if !task.reconciled {
			if err := insertEvent(ctx, tx, Event{TaskID: task.id, Kind: "task.restored_reconcile", Actor: "eng.restore", Data: map[string]any{"previous_state": task.state, "automatic_replay": false, "publication_requires_readback": true}}); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// AcceptTask records a user's acceptance of a completed local result for a
// project with no configured checks. It cannot clear missing checks, a failed
// command, an unknown external outcome, or an unresolved local process.
func (store *Store) AcceptTask(ctx context.Context, taskID, note, actor string) (Task, error) {
	if strings.TrimSpace(note) == "" || len(note) > 4096 || strings.TrimSpace(actor) == "" || len(actor) > 256 {
		return Task{}, fmt.Errorf("acceptance requires a concise note and an actor")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", taskID))
	if err != nil {
		return Task{}, err
	}
	if task.State != StateWaiting || task.CurrentAttempt < 1 || task.StopRequested || len(task.Config.Project.Checks) != 0 {
		return Task{}, ErrInvalidState
	}
	attempt, err := scanAttempt(tx.QueryRowContext(ctx, "SELECT "+attemptColumns+" FROM attempts WHERE task_id=? AND number=?", task.ID, task.CurrentAttempt))
	if err != nil {
		return Task{}, err
	}
	if attempt.State != StateWaiting || !attempt.RuntimeStarted || !attempt.Result.RuntimeStarted || !attempt.Result.LocalProcessStopped || attempt.Result.RuntimeExitCode != 0 || attempt.Result.RemoteOutcomeUnknown || !attempt.Result.ManualAcceptanceReady || len(attempt.Result.Checks) != 0 || attempt.Result.ChecksStatus != "not_configured" || !validCommit(attempt.Result.ResultCommit) {
		return Task{}, ErrInvalidState
	}
	task.State, task.Stage, task.Reason = StateDone, StageDeliver, "accepted_without_configured_checks"
	task.UpdatedAt = time.Now().UTC()
	attempt.State, attempt.Stage, attempt.Reason = task.State, task.Stage, task.Reason
	attempt.Result.Status, attempt.Result.Reason = task.State, task.Reason
	if err := persistAttempt(ctx, tx, attempt); err != nil {
		return Task{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET state=?,stage=?,reason=?,updated_at=? WHERE id=?", task.State, task.Stage, task.Reason, formatTime(task.UpdatedAt), task.ID); err != nil {
		return Task{}, err
	}
	if err := insertEvent(ctx, tx, Event{TaskID: task.ID, AttemptID: attempt.ID, Kind: "task.accepted", Actor: actor, Data: map[string]any{"note": note, "checks_status": "not_configured", "result_commit": attempt.Result.ResultCommit}}); err != nil {
		return Task{}, err
	}
	return task, tx.Commit()
}

func (store *Store) AddEvent(ctx context.Context, event Event) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if event.AttemptID != "" {
		var taskID string
		if err := tx.QueryRowContext(ctx, "SELECT task_id FROM attempts WHERE id=?", event.AttemptID).Scan(&taskID); err != nil {
			return ErrNotFound
		}
		if taskID != event.TaskID {
			return ErrConflict
		}
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func insertEvent(ctx context.Context, tx *sql.Tx, event Event) error {
	if event.TaskID == "" || !configName.MatchString(event.Kind) || len(event.Actor) > 256 || strings.ContainsRune(event.Actor, '\x00') {
		return fmt.Errorf("invalid event identity")
	}
	if event.Actor == "" {
		event.Actor = "local"
	}
	data, err := encodeJSON(event.Data)
	if err != nil {
		return err
	}
	var attempt any
	if event.AttemptID != "" {
		attempt = event.AttemptID
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO events(task_id,attempt_id,kind,actor,data_json,created_at) VALUES(?,?,?,?,?,?)", event.TaskID, attempt, event.Kind, event.Actor, data, formatTime(time.Now().UTC()))
	return err
}

func (store *Store) ListEvents(ctx context.Context, taskID string) ([]Event, error) {
	rows, err := store.db.QueryContext(ctx, "SELECT id,task_id,COALESCE(attempt_id,''),kind,actor,data_json,created_at FROM events WHERE task_id=? ORDER BY id", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var data, created string
		if err := rows.Scan(&event.ID, &event.TaskID, &event.AttemptID, &event.Kind, &event.Actor, &data, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &event.Data); err != nil {
			return nil, fmt.Errorf("invalid stored event data")
		}
		if event.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("invalid stored event timestamp")
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func sameTaskInput(a, b Task) bool {
	return a.Project == b.Project && a.Owner == b.Owner && a.Instruction == b.Instruction && a.ConfigDigest == b.ConfigDigest && jsonEqual(a.Config, b.Config) && a.CreatedAt.Equal(b.CreatedAt)
}

func sameAttemptInput(a, b Attempt) bool {
	return a.ID == b.ID && a.TaskID == b.TaskID && a.Number == b.Number && a.Input == b.Input && a.ConfigDigest == b.ConfigDigest && jsonEqual(a.Config, b.Config) && a.StartedAt.Equal(b.StartedAt)
}

func changedNonempty(old, next string) bool { return old != "" && old != next }

func validCommit(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func stageRank(stage string) int {
	switch stage {
	case StagePrepare:
		return 0
	case StageExecute:
		return 1
	case StageVerify:
		return 2
	case StageDeliver:
		return 3
	}
	return -1
}

func encodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(data) > maxConfigBytes {
		return "", fmt.Errorf("record exceeds 1 MiB")
	}
	return string(data), nil
}

func jsonEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && string(left) == string(right)
}

func formatTime(value time.Time) string { return value.UTC().Format(storedTime) }
