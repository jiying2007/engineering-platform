package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunResult describes observed local work. It never claims model quality,
// device verification, zero cost, or remote non-execution from local exit alone.
type RunResult struct {
	Status                string        `json:"status"`
	Reason                string        `json:"reason,omitempty"`
	BaseCommit            string        `json:"base_commit,omitempty"`
	StartCommit           string        `json:"start_commit,omitempty"`
	ResultCommit          string        `json:"result_commit,omitempty"`
	Branch                string        `json:"branch,omitempty"`
	Worktree              string        `json:"worktree,omitempty"`
	DiffPath              string        `json:"diff_path,omitempty"`
	SummaryPath           string        `json:"summary_path,omitempty"`
	RuntimeStdout         string        `json:"runtime_stdout,omitempty"`
	RuntimeStderr         string        `json:"runtime_stderr,omitempty"`
	RuntimeExitCode       int           `json:"runtime_exit_code"`
	RuntimeStarted        bool          `json:"runtime_started"`
	LocalProcessStopped   bool          `json:"local_process_stopped"`
	RemoteOutcomeUnknown  bool          `json:"remote_outcome_unknown"`
	ManualAcceptanceReady bool          `json:"manual_acceptance_ready"`
	ChecksStatus          string        `json:"checks_status"`
	Checks                []CheckResult `json:"checks"`
	Artifacts             []Artifact    `json:"artifacts"`
	Usage                 Usage         `json:"usage"`
}

var errStopRequested = errors.New("user requested local execution stop")

// Execute performs one queued attempt, under an already-held scheduler lock.
// It never performs a model retry, automatically resumes, pushes or opens a PR.
func Execute(ctx context.Context, store *Store, taskID string) (finished Task, finalErr error) {
	if store == nil {
		return Task{}, fmt.Errorf("Execute requires the single scheduler lock")
	}
	releaseOperation, err := beginSchedulerOperation(store.DataDir())
	if err != nil {
		return Task{}, err
	}
	defer releaseOperation()
	if err := ensureExecutionSlot(ctx, store); err != nil {
		return Task{}, err
	}
	task, err := store.GetTask(ctx, taskID)
	if err != nil {
		return task, err
	}
	repoLock, _, err := AcquireRepository(ctx, task.Config.Project.Root)
	if err != nil {
		return task, err
	}
	defer repoLock.Close()
	task, attempt, err := store.ClaimTask(ctx, taskID)
	if err != nil {
		return task, err
	}
	result := RunResult{Status: StateFailed, RuntimeExitCode: -1, LocalProcessStopped: true, ChecksStatus: "incomplete", Checks: []CheckResult{}, Artifacts: []Artifact{}, Usage: Usage{Known: false, Note: "Runtime usage was not reported; cost is unknown, not zero."}}
	defer func() {
		if result.LocalProcessStopped {
			if err := writeLockRecord(repoLock, nil); err != nil {
				finalErr = errors.Join(finalErr, fmt.Errorf("clear stopped repository process record: %w", err))
			}
		}
	}()
	if !attempt.Config.Runtime.Model {
		result.Usage.Note = "Configured non-model command; this run is not evidence of model quality or model usage."
	}
	attempt.RuntimeVersion = "unknown (not probed)"
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	monitorDone := make(chan struct{})
	go monitorStop(runCtx, store, task.ID, cancel, monitorDone)
	defer func() { cancel(nil); <-monitorDone }()
	taskDir, pathErr := taskDirectory(store.DataDir(), task.ID)
	if pathErr != nil {
		return finishExecution(store, task, attempt, result, StateFailed, pathErr.Error(), "")
	}
	attemptDir := filepath.Join(taskDir, fmt.Sprintf("attempt-%d", attempt.Number))
	if err := os.Mkdir(attemptDir, 0700); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(taskDir, 0700)
			if err == nil {
				err = os.Mkdir(attemptDir, 0700)
			}
		}
		if err != nil {
			return finishExecution(store, task, attempt, result, StateWaiting, "cannot create new attempt directory: "+err.Error(), "")
		}
	}
	taskFile := filepath.Join(attemptDir, "task.md")
	if err := writeExclusive(taskFile, []byte(attempt.Input)); err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, err.Error(), attemptDir)
	}
	if err := prepareWorkspace(runCtx, store, &task); err != nil {
		state := StateFailed
		if runCtx.Err() != nil {
			state = StateCancelled
		}
		return finishExecution(store, task, attempt, result, state, "prepare workspace: "+err.Error(), attemptDir)
	}
	result.BaseCommit, result.Branch, result.Worktree = task.BaseSHA, task.Branch, task.Worktree
	start, err := snapshotWorkspace(runCtx, task, fmt.Sprintf("eng: preserve input for %s attempt %d", task.ID, attempt.Number))
	if err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, "cannot preserve exact input: "+err.Error(), attemptDir)
	}
	attempt.StartCommit, result.StartCommit = start, start
	task.Stage, attempt.Stage = StageExecute, StageExecute
	attempt.Result = result
	if err := store.UpdateTask(runCtx, task); err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, err.Error(), attemptDir)
	}
	if err := store.UpdateAttempt(runCtx, attempt); err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, err.Error(), attemptDir)
	}
	result.RuntimeStdout, result.RuntimeStderr = filepath.Join(attemptDir, "runtime.stdout.log"), filepath.Join(attemptDir, "runtime.stderr.log")
	env := executionEnvironment(taskFile, task.Worktree)
	inherited := []*os.File{schedulerFile(store.DataDir()), repoLock.file}
	persist := func(id ProcessIdentity) error {
		attempt.Process = &id
		attempt.RuntimeStarted = true
		result.LocalProcessStopped = false
		attempt.Result = result
		if err := store.UpdateAttempt(runCtx, attempt); err != nil {
			return err
		}
		return writeLockRecord(repoLock, id)
	}
	duration, err := time.ParseDuration(attempt.Config.Runtime.Timeout)
	if err != nil || duration <= 0 {
		return finishExecution(store, task, attempt, result, StateFailed, "invalid frozen runtime timeout", attemptDir)
	}
	input := attempt.Input
	if attempt.Config.Runtime.Stdin == "none" {
		input = ""
	}
	commandCtx, commandCancel := context.WithTimeout(runCtx, duration)
	out := runSupervised(commandCtx, attempt.Config.Runtime.Argv, task.Worktree, input, env, result.RuntimeStdout, result.RuntimeStderr, inherited, persist)
	commandCancel()
	result.RuntimeStarted, result.RuntimeExitCode, result.LocalProcessStopped = out.Started, out.ExitCode, out.Stopped
	result.RemoteOutcomeUnknown = attempt.Config.Runtime.Model && out.Started && (out.TimedOut || out.Cancelled || !out.Stopped)
	if out.Identity != nil {
		attempt.Process = out.Identity
	}
	if out.Err != nil || out.ExitCode != 0 || !out.Stopped {
		state, reason := StateFailed, fmt.Sprintf("configured runtime exited %d", out.ExitCode)
		if out.Err != nil {
			reason = out.Err.Error()
		}
		if out.TimedOut {
			reason = "configured runtime exceeded its deadline; no automatic retry"
		}
		if !out.Stopped || (out.TimedOut && attempt.Config.Runtime.Model) {
			state = StateWaiting
		}
		if out.Cancelled && out.Stopped {
			state = StateCancelled
			reason = "local execution stopped; remote model execution or cost may remain unknown"
		}
		if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, errStopRequested) {
			state = StateWaiting
			reason = cause.Error()
		}
		return finishExecution(store, task, attempt, result, state, reason, attemptDir)
	}
	if err := verifyWorkspace(runCtx, task); err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, err.Error(), attemptDir)
	}
	result.ResultCommit, err = snapshotWorkspace(runCtx, task, fmt.Sprintf("eng: result for %s attempt %d", task.ID, attempt.Number))
	if err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, "capture candidate: "+err.Error(), attemptDir)
	}
	task.Stage, attempt.Stage = StageVerify, StageVerify
	attempt.Result = result
	if err := store.UpdateTask(runCtx, task); err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, err.Error(), attemptDir)
	}
	if err := store.UpdateAttempt(runCtx, attempt); err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, err.Error(), attemptDir)
	}
	for i, check := range attempt.Config.Project.Checks {
		item := CheckResult{Name: check.Name, Argv: append([]string(nil), check.Argv...), SourceCommit: result.ResultCommit, ExitCode: -1, Status: "failed", StartedAt: time.Now().UTC(), Stdout: filepath.Join(attemptDir, fmt.Sprintf("check-%02d-%s.stdout.log", i, check.Name)), Stderr: filepath.Join(attemptDir, fmt.Sprintf("check-%02d-%s.stderr.log", i, check.Name))}
		duration, err := time.ParseDuration(check.Timeout)
		if err != nil || duration <= 0 {
			return finishExecution(store, task, attempt, result, StateFailed, "invalid frozen check timeout", attemptDir)
		}
		checkCtx, checkCancel := context.WithTimeout(runCtx, duration)
		checkOutcome := runSupervised(checkCtx, check.Argv, task.Worktree, "", env, item.Stdout, item.Stderr, inherited, persist)
		checkCancel()
		item.ExitCode, item.EndedAt = checkOutcome.ExitCode, time.Now().UTC()
		result.LocalProcessStopped = checkOutcome.Stopped
		if checkOutcome.Identity != nil {
			attempt.Process = checkOutcome.Identity
		}
		if checkOutcome.Err == nil && checkOutcome.ExitCode == 0 && checkOutcome.Stopped {
			item.Status = "passed"
		}
		if checkOutcome.TimedOut {
			item.Status = "timeout"
		}
		if checkOutcome.Cancelled {
			item.Status = "cancelled"
		}
		result.Checks = append(result.Checks, item)
		attempt.Result = result
		if err := store.UpdateAttempt(runCtx, attempt); err != nil {
			return finishExecution(store, task, attempt, result, StateWaiting, "record check result: "+err.Error(), attemptDir)
		}
		if item.Status != "passed" {
			result.ChecksStatus = "failed"
			state := StateFailed
			if !checkOutcome.Stopped {
				state = StateWaiting
			}
			if checkOutcome.Cancelled && checkOutcome.Stopped {
				state = StateCancelled
			}
			return finishExecution(store, task, attempt, result, state, fmt.Sprintf("required check %s %s (exit %d)", check.Name, item.Status, item.ExitCode), attemptDir)
		}
		changed, err := trackedWorkspaceChanged(runCtx, task, result.ResultCommit)
		if err != nil || changed {
			result.ChecksStatus = "incomplete"
			return finishExecution(store, task, attempt, result, StateWaiting, "checks changed source or Git identity; checks do not verify the new bytes", attemptDir)
		}
	}
	result.ChecksStatus = "passed"
	if len(attempt.Config.Project.Checks) == 0 {
		result.ChecksStatus = "not_configured"
	}
	result.Artifacts, err = captureArtifacts(runCtx, task, filepath.Join(attemptDir, "artifacts"))
	if err != nil {
		return finishExecution(store, task, attempt, result, StateWaiting, "required artifacts: "+err.Error(), attemptDir)
	}
	if len(attempt.Config.Project.Checks) == 0 {
		result.ManualAcceptanceReady = true
		return finishExecution(store, task, attempt, result, StateWaiting, "No checks configured; local result is ready for manual review", attemptDir)
	}
	return finishExecution(store, task, attempt, result, StateDone, "configured checks and local artifact capture completed; semantic acceptance remains with the task owner", attemptDir)
}

func executionEnvironment(taskFile, workspace string) []string {
	env := []string{}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN", "GLAB_TOKEN", "CI_JOB_TOKEN", "ENG_TASK_FILE", "ENG_WORKSPACE", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE":
			continue
		}
		env = append(env, value)
	}
	// Environment hygiene is not an OS sandbox; same-UID credential files remain
	// available in trusted local mode, as the user-facing deployment contract says.
	return append(env, "ENG_TASK_FILE="+taskFile, "ENG_WORKSPACE="+workspace)
}

func monitorStop(ctx context.Context, store *Store, taskID string, cancel context.CancelCauseFunc, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		queryCtx, finish := context.WithTimeout(ctx, 2*time.Second)
		stopped, err := store.StopRequested(queryCtx, taskID)
		finish()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			cancel(fmt.Errorf("cannot observe task control state; execution stopped: %w", err))
			return
		}
		if stopped {
			cancel(errStopRequested)
			return
		}
	}
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr, closeErr := file.Sync(), file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func finishExecution(store *Store, task Task, attempt Attempt, result RunResult, state, reason, attemptDir string) (Task, error) {
	// Cancellation of the run must not cancel its bounded settlement.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if stop, err := store.StopRequested(ctx, task.ID); err != nil {
		state, result.ManualAcceptanceReady = StateWaiting, false
		reason += "; final stop state could not be observed: " + err.Error()
	} else if stop && result.LocalProcessStopped {
		state, result.ManualAcceptanceReady = StateCancelled, false
		reason = "stop requested; local execution has stopped; retained work is available for review"
	}
	if result.LocalProcessStopped && task.Worktree != "" && attemptDir != "" {
		if result.ResultCommit == "" && attempt.RuntimeStarted {
			commit, err := snapshotWorkspace(ctx, task, fmt.Sprintf("eng: retained checkpoint for %s attempt %d", task.ID, attempt.Number))
			if err == nil {
				result.ResultCommit = commit
			} else {
				state = StateWaiting
				result.ManualAcceptanceReady = false
				reason += "; source checkpoint unavailable: " + err.Error()
			}
		}
		if result.ResultCommit != "" {
			result.DiffPath = filepath.Join(attemptDir, "changes.patch")
			if err := writeWorkspaceDiff(ctx, task, result.ResultCommit, result.DiffPath); err != nil {
				state = StateWaiting
				result.ManualAcceptanceReady = false
				reason += "; diff retention failed: " + err.Error()
			}
		}
	}
	result.Status, result.Reason = state, reason
	result.BaseCommit, result.StartCommit, result.Branch, result.Worktree = task.BaseSHA, attempt.StartCommit, task.Branch, task.Worktree
	task.State, task.Reason = state, reason
	if state == StateDone {
		task.Stage = StageDeliver
	}
	attempt.State, attempt.Stage, attempt.Reason, attempt.EndedAt = state, task.Stage, reason, time.Now().UTC()
	if attemptDir != "" {
		result.SummaryPath = filepath.Join(attemptDir, "summary.md")
		summary := fmt.Sprintf("# Task %s — attempt %d\n\nState: %s\n\n%s\n\nBase: `%s`\n\nInput: `%s`\n\nResult: `%s`\n\nRuntime exit: %d\n\nChecks: %s (%d executed)\n\nLocal process stopped: %t\n\nRemote outcome unknown: %t\n\nUsage: %s\n\nThis is a local execution record, not independent review, device verification, or proof that the requested behavior is correct.\n", task.ID, attempt.Number, state, reason, task.BaseSHA, attempt.StartCommit, result.ResultCommit, result.RuntimeExitCode, result.ChecksStatus, len(result.Checks), result.LocalProcessStopped, result.RemoteOutcomeUnknown, result.Usage.Note)
		summary += "\nThis file is an execution snapshot. Use `eng status " + task.ID + "` for current acceptance, recovery and publication facts.\n\n## Task input\n\n" + attempt.Input + "\n\n## Executed checks\n\n"
		for _, check := range result.Checks {
			argv, _ := json.Marshal(check.Argv)
			summary += fmt.Sprintf("- %s: %s; exit %d; source `%s`; argv `%s`; stdout `%s`; stderr `%s`\n", check.Name, check.Status, check.ExitCode, check.SourceCommit, argv, check.Stdout, check.Stderr)
		}
		if len(result.Checks) == 0 {
			summary += "No configured check was executed.\n"
		}
		summary += "\n## Retained artifacts\n\n"
		for _, artifact := range result.Artifacts {
			summary += fmt.Sprintf("- `%s` — %d bytes — `%s`\n", artifact.Path, artifact.Size, artifact.Digest)
		}
		if len(result.Artifacts) == 0 {
			summary += "No artifacts were captured.\n"
		}
		if err := writeExclusive(result.SummaryPath, []byte(summary)); err != nil {
			task.State, attempt.State, result.Status = StateWaiting, StateWaiting, StateWaiting
			result.ManualAcceptanceReady = false
			reason += "; summary retention failed: " + err.Error()
		}
		result.Reason = reason
		envelope := struct {
			TaskID       string    `json:"task_id"`
			AttemptID    string    `json:"attempt_id"`
			ConfigDigest string    `json:"config_digest"`
			Result       RunResult `json:"result"`
		}{task.ID, attempt.ID, attempt.ConfigDigest, result}
		raw, err := json.MarshalIndent(envelope, "", "  ")
		if err == nil {
			err = writeExclusive(filepath.Join(attemptDir, "result.json"), append(raw, '\n'))
		}
		if err != nil {
			task.State, attempt.State, result.Status = StateWaiting, StateWaiting, StateWaiting
			result.ManualAcceptanceReady = false
			reason += "; result retention failed: " + err.Error()
		}
	}
	task.Reason, attempt.Reason, result.Reason = reason, reason, reason
	attempt.Result = result
	if err := store.FinishAttempt(ctx, task, attempt); err != nil {
		// A stop can race the final artifact writes. SQLite prevents success after
		// the stop request; settle that one race as cancelled without replaying.
		if task.State == StateDone && errors.Is(err, ErrInvalidState) {
			current, readErr := store.GetTask(ctx, task.ID)
			if readErr == nil && current.State == StateRunning && current.StopRequested && result.LocalProcessStopped {
				task.State, attempt.State, result.Status = StateCancelled, StateCancelled, StateCancelled
				task.Reason = "stop requested during final settlement; retained output is not accepted as done"
				attempt.Reason, result.Reason = task.Reason, task.Reason
				result.ManualAcceptanceReady = false
				attempt.Result = result
				if settleErr := store.FinishAttempt(ctx, task, attempt); settleErr == nil {
					return store.GetTask(ctx, task.ID)
				}
			}
		}
		return task, fmt.Errorf("execution stopped but final state could not be recorded; do not retry blindly: %w", err)
	}
	return store.GetTask(ctx, task.ID)
}

func ensureExecutionSlot(ctx context.Context, store *Store) error {
	if err := ensurePublicationStopped(ctx, store); err != nil {
		return err
	}
	tasks, err := store.ListUnfinished(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		attempts, err := store.ListAttempts(ctx, task.ID)
		if err != nil {
			return err
		}
		for _, attempt := range attempts {
			if attempt.Process == nil || attempt.Result.LocalProcessStopped {
				continue
			}
			alive, err := ProcessGroupAlive(*attempt.Process)
			if err != nil || alive {
				return fmt.Errorf("%w: task %s attempt %d retains the global execution slot", ErrProcessUnresolved, task.ID, attempt.Number)
			}
		}
	}
	return nil
}

// Recover observes interrupted attempts and records a bounded waiting result.
// No saved Runtime command is invoked, and retained JSON cannot promote success.
func Recover(ctx context.Context, store *Store) ([]Task, error) {
	if store == nil {
		return nil, fmt.Errorf("Recover requires the scheduler lock")
	}
	releaseOperation, err := beginSchedulerOperation(store.DataDir())
	if err != nil {
		return nil, err
	}
	defer releaseOperation()
	tasks, err := store.ListUnfinished(ctx)
	if err != nil {
		return nil, err
	}
	recovered := []Task{}
	for _, task := range tasks {
		if task.State != StateRunning && task.State != StateWaiting {
			continue
		}
		attempts, err := store.ListAttempts(ctx, task.ID)
		if err != nil {
			return recovered, err
		}
		if len(attempts) == 0 {
			if task.State == StateWaiting && task.CurrentAttempt == 0 {
				continue
			}
			return recovered, fmt.Errorf("interrupted task %s has no attempt", task.ID)
		}
		attempt := attempts[len(attempts)-1]
		if task.State == StateWaiting && (attempt.Process == nil || attempt.Result.LocalProcessStopped) {
			continue
		}
		result := attempt.Result
		result.ManualAcceptanceReady = false
		result.Status, result.Reason = StateWaiting, "supervisor interrupted; retained files require review; runtime was not replayed"
		result.LocalProcessStopped = attempt.Process == nil
		if attempt.Process != nil {
			alive, observeErr := ProcessGroupAlive(*attempt.Process)
			result.LocalProcessStopped = observeErr == nil && !alive
			if observeErr != nil {
				result.Reason += "; process identity cannot be confirmed: " + observeErr.Error()
			}
			if alive {
				result.Reason += "; original local process group remains active and occupies its slot"
			}
		}
		if attempt.Config.Runtime.Model && attempt.RuntimeStarted {
			result.RemoteOutcomeUnknown = true
		}
		if task.State == StateWaiting && jsonEqual(attempt.Result, result) {
			continue // observation polling is not a new lifecycle event
		}
		task.State, task.Reason = StateWaiting, result.Reason
		attempt.State, attempt.Reason, attempt.EndedAt, attempt.Result = StateWaiting, result.Reason, time.Now().UTC(), result
		if err := store.FinishAttempt(ctx, task, attempt); err != nil {
			return recovered, err
		}
		recovered = append(recovered, task)
	}
	return recovered, nil
}

// StopTask requests cooperative stop from a live owner. After a supervisor has
// gone, an explicit stop may terminate only an exactly identified local group.
func StopTask(ctx context.Context, store *Store, taskID string) error {
	if err := store.RequestStop(ctx, taskID); err != nil {
		return err
	}
	task, err := store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.State != StateWaiting && task.State != StateRunning {
		return nil
	}
	if task.State == StateRunning {
		alive, err := schedulerOwnerAlive(store.DataDir())
		if err != nil || alive {
			return err
		}
	}
	attempts, err := store.ListAttempts(ctx, task.ID)
	if err != nil || len(attempts) == 0 {
		return err
	}
	attempt := attempts[len(attempts)-1]
	if attempt.Process != nil && !attempt.Result.LocalProcessStopped {
		stopped, err := stopProcessGroup(*attempt.Process, 500*time.Millisecond)
		if err != nil {
			return err
		}
		if !stopped {
			return ErrProcessUnresolved
		}
	}
	attempt.Result.LocalProcessStopped = true
	attempt.Result.ManualAcceptanceReady = false
	attempt.Result.Status, task.State, attempt.State = StateWaiting, StateWaiting, StateWaiting
	attempt.Result.RemoteOutcomeUnknown = attempt.Result.RemoteOutcomeUnknown || attempt.Config.Runtime.Model && attempt.RuntimeStarted
	attempt.Result.Reason = "operator confirmed original local execution stopped; interrupted work remains available for review"
	attempt.Reason, task.Reason = attempt.Result.Reason, attempt.Result.Reason
	// Keep waiting and remote-outcome uncertainty. Stopping local execution is
	// not permission to reinterpret an interrupted model request as never sent.
	return store.FinishAttempt(ctx, task, attempt)
}
