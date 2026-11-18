package local

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newStoreForTest(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createStoreTask(t *testing.T, store *Store, checks []Check) Task {
	t.Helper()
	config, digest, err := Freeze(Project{Name: "demo", Root: t.TempDir(), Base: "main", Runtime: "fixture", Checks: checks}, Runtime{Name: "fixture", Argv: []string{"/bin/true"}, Stdin: "task", Timeout: "5s", Model: false})
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.CreateTask(context.Background(), Task{Project: "demo", Owner: "tester", Instruction: "Prepare a reviewable local change", Config: config, ConfigDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// Two real SQLite connections race the same queued task. Only the winner may
// create an attempt; all losing claims must leave neither attempt nor event.
func TestStoreAtomicClaimAcrossConnections(t *testing.T) {
	first := newStoreForTest(t)
	second, err := OpenStore(first.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	task := createStoreTask(t, first, nil)
	start := make(chan struct{})
	results := make(chan error, 20)
	var workers sync.WaitGroup
	for i := range 20 {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			store := first
			if i%2 != 0 {
				store = second
			}
			_, _, err := store.ClaimTask(context.Background(), task.ID)
			results <- err
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrInvalidState) {
			t.Fatalf("unexpected claim failure: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d successful claimants, want one", winners)
	}
	attempts, err := second.ListAttempts(context.Background(), task.ID)
	if err != nil || len(attempts) != 1 || attempts[0].Number != 1 {
		t.Fatalf("attempts after race: %+v, %v", attempts, err)
	}
	events, err := second.ListEvents(context.Background(), task.ID)
	if err != nil || len(events) != 2 || events[1].Kind != "attempt.claimed" {
		t.Fatalf("race wrote extra events: %+v, %v", events, err)
	}
}

func TestStoreFrozenInputsAndExplicitResume(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)
	submitted := createStoreTask(t, store, nil)
	submitted.Config.Runtime.Argv[0] = "/bin/false"
	task, attempt, err := store.ClaimTask(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Config.Runtime.Argv[0] != "/bin/true" {
		t.Fatal("caller mutation changed frozen stored config")
	}
	attempt.StartCommit = strings.Repeat("a", 40)
	if err := store.UpdateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	attempt.RuntimeStarted = true
	attempt.Process = &ProcessIdentity{PID: 4321, PGID: 4321, StartTicks: 10, BootID: "fixture-boot"}
	attempt.Result.LocalProcessStopped = false
	if err := store.UpdateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	bad := attempt
	bad.Input = "different frozen task"
	if err := store.UpdateAttempt(ctx, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("input mutation accepted: %v", err)
	}
	bad = attempt
	bad.StartCommit = strings.Repeat("b", 40)
	if err := store.UpdateAttempt(ctx, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("starting commit mutation accepted: %v", err)
	}
	task.State, task.Reason = StateWaiting, "process_status_unknown"
	if err := store.FinishAttempt(ctx, task, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueResume(ctx, task.ID, ""); !errors.Is(err, ErrProcessUnresolved) {
		t.Fatalf("unresolved process was requeued: %v", err)
	}
	attempt, err = store.GetAttempt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !attempt.EndedAt.IsZero() {
		t.Fatal("unknown local process was recorded as ended")
	}
	attempt.Result.LocalProcessStopped = true
	if err := store.UpdateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueResume(ctx, task.ID, "Continue using the preserved workspace"); err != nil {
		t.Fatal(err)
	}
	_, next, err := store.ClaimTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := store.GetAttempt(ctx, attempt.ID)
	if next.Number != 2 || next.Input == old.Input || next.ConfigDigest != old.ConfigDigest || old.Input != "Prepare a reviewable local change" || next.StartCommit != "" {
		t.Fatalf("resume did not preserve and separate attempts: old=%+v new=%+v", old, next)
	}
}

func TestStoreStopCannotBeLostByStaleProgress(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)
	queued := createStoreTask(t, store, nil)
	if err := store.RequestStop(ctx, queued.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ClaimTask(ctx, queued.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cancelled queue entry was claimed: %v", err)
	}
	if attempts, _ := store.ListAttempts(ctx, queued.ID); len(attempts) != 0 {
		t.Fatal("cancel-before-start created an attempt")
	}
	task := createStoreTask(t, store, nil)
	task, attempt, err := store.ClaimTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RequestStop(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	task.Stage = StageExecute // this in-memory task still has StopRequested=false
	if err := store.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	requested, err := store.StopRequested(ctx, task.ID)
	if err != nil || !requested {
		t.Fatalf("stale progress lost cancellation: %v, %v", requested, err)
	}
	task.State = StateDone
	attempt.Result.LocalProcessStopped = true
	if err := store.FinishAttempt(ctx, task, attempt); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("stop request was hidden by done result: %v", err)
	}
	task.State, task.Reason = StateCancelled, "local execution stopped"
	if err := store.FinishAttempt(ctx, task, attempt); err != nil {
		t.Fatal(err)
	}
}

// A failure while retaining the completion event must roll back both state and
// result. This uses SQLite's real transaction, not a mock store or mirror model.
func TestStoreFinishAndEventAreOneTransaction(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)
	task := createStoreTask(t, store, nil)
	task, attempt, err := store.ClaimTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.ExecContext(ctx, `CREATE TRIGGER fail_finish_event BEFORE INSERT ON events
 WHEN NEW.kind='attempt.finished' BEGIN SELECT RAISE(ABORT,'injected event write failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	task.State, task.Reason = StateFailed, "observed runtime failure"
	attempt.Result = RunResult{RuntimeExitCode: 3, LocalProcessStopped: true}
	if err := store.FinishAttempt(ctx, task, attempt); err == nil {
		t.Fatal("injected write failure was ignored")
	}
	gotTask, _ := store.GetTask(ctx, task.ID)
	gotAttempt, _ := store.GetAttempt(ctx, attempt.ID)
	if gotTask.State != StateRunning || gotAttempt.State != StateRunning || gotAttempt.Result.RuntimeExitCode == 3 {
		t.Fatal("partial terminal state escaped a rolled-back transaction")
	}
	if _, err := store.db.ExecContext(ctx, "DROP TRIGGER fail_finish_event"); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishAttempt(ctx, task, attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishAttempt(ctx, task, attempt); err != nil {
		t.Fatalf("exact repeated completion should be idempotent: %v", err)
	}
	events, _ := store.ListEvents(ctx, task.ID)
	if len(events) != 3 {
		t.Fatalf("completion retained %d events, want three", len(events))
	}
}

func TestStoreDurableReopenAndSchemaRefusal(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)
	task := createStoreTask(t, store, nil)
	path := store.DataDir()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetTask(ctx, task.ID)
	if err != nil || got.ConfigDigest != task.ConfigDigest || got.State != StateQueued {
		t.Fatalf("reopened state: %+v %v", got, err)
	}
	var busy, synchronous, foreignKeys int
	_ = reopened.db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy)
	_ = reopened.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous)
	_ = reopened.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys)
	if busy != 5000 || synchronous != 2 || foreignKeys != 1 || reopened.db.Stats().MaxOpenConnections != 1 {
		t.Fatal("database reopened without bounded, durable connection settings")
	}
	if _, err := reopened.db.ExecContext(ctx, "PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	if wrong, err := OpenStore(path); err == nil {
		wrong.Close()
		t.Fatal("older binary opened a future schema")
	}
}

func TestStoreCancelledContextCreatesNothing(t *testing.T) {
	store := newStoreForTest(t)
	task := createStoreTask(t, store, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := store.ClaimTask(ctx, task.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("claim ignored cancellation: %v", err)
	}
	attempts, _ := store.ListAttempts(context.Background(), task.ID)
	got, _ := store.GetTask(context.Background(), task.ID)
	if len(attempts) != 0 || got.State != StateQueued {
		t.Fatal("cancelled claim changed durable state")
	}
}

func TestStoreManualAcceptanceDoesNotInventChecks(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)
	task := createStoreTask(t, store, nil)
	task, attempt, err := store.ClaimTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	attempt.StartCommit = strings.Repeat("a", 40)
	if err := store.UpdateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	attempt.RuntimeStarted = true
	attempt.Result = RunResult{RuntimeStarted: true, RuntimeExitCode: 0, LocalProcessStopped: true, ChecksStatus: "not_configured", ManualAcceptanceReady: true, ResultCommit: strings.Repeat("b", 40)}
	task.State, task.Stage, task.Reason = StateWaiting, StageDeliver, "manual review required"
	if err := store.FinishAttempt(ctx, task, attempt); err != nil {
		t.Fatal(err)
	}
	accepted, err := store.AcceptTask(ctx, task.ID, "Reviewed the documentation change", "alice")
	if err != nil || accepted.State != StateDone {
		t.Fatalf("acceptance: %+v %v", accepted, err)
	}
	got, _ := store.GetAttempt(ctx, attempt.ID)
	if len(got.Result.Checks) != 0 || got.Result.ChecksStatus != "not_configured" {
		t.Fatal("manual acceptance fabricated a test result")
	}
	events, _ := store.ListEvents(ctx, task.ID)
	last := events[len(events)-1]
	if last.Kind != "task.accepted" || last.Actor != "alice" || last.Data["note"] != "Reviewed the documentation change" {
		t.Fatalf("acceptance attribution missing: %+v", last)
	}
}

func TestStoreAcceptanceRejectsIncompleteOrUnknownResults(t *testing.T) {
	for _, scenario := range []string{"not_started", "failed", "remote_unknown", "process_live", "required_checks", "artifacts_incomplete"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			store := newStoreForTest(t)
			var checks []Check
			if scenario == "required_checks" {
				checks = []Check{{Name: "test", Argv: []string{"/bin/true"}, Timeout: "5s"}}
			}
			task := createStoreTask(t, store, checks)
			task, attempt, err := store.ClaimTask(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			attempt.RuntimeStarted = true
			attempt.Result = RunResult{RuntimeStarted: true, RuntimeExitCode: 0, LocalProcessStopped: true, ChecksStatus: "not_configured", ManualAcceptanceReady: true, ResultCommit: strings.Repeat("a", 40)}
			switch scenario {
			case "not_started":
				attempt.RuntimeStarted = false
				attempt.Result.RuntimeStarted = false
			case "failed":
				attempt.Result.RuntimeExitCode = 2
			case "remote_unknown":
				attempt.Result.RemoteOutcomeUnknown = true
			case "process_live":
				attempt.Result.LocalProcessStopped = false
			case "artifacts_incomplete":
				attempt.Result.ManualAcceptanceReady = false
			}
			task.State = StateWaiting
			if err := store.FinishAttempt(ctx, task, attempt); err != nil {
				t.Fatal(err)
			}
			if _, err := store.AcceptTask(ctx, task.ID, "Reviewed", "alice"); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("unsafe acceptance succeeded: %v", err)
			}
		})
	}
}

func TestStoreEventCannotBorrowAnotherTaskAttempt(t *testing.T) {
	store := newStoreForTest(t)
	ctx := context.Background()
	first := createStoreTask(t, store, nil)
	second := createStoreTask(t, store, nil)
	_, attempt, err := store.ClaimTask(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = store.AddEvent(ctx, Event{TaskID: second.ID, AttemptID: attempt.ID, Kind: "publish.intent", Actor: "tester", CreatedAt: time.Now().UTC()})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-task event accepted: %v", err)
	}
}

func TestStoreRestoredQueueRequiresExplicitResume(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)
	queued := createStoreTask(t, store, nil)
	running := createStoreTask(t, store, nil)
	if _, _, err := store.ClaimTask(ctx, running.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRestored(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRestored(ctx); err != nil {
		t.Fatal(err)
	}
	paused, err := store.GetTask(ctx, queued.ID)
	if err != nil || paused.State != StateWaiting || paused.CurrentAttempt != 0 {
		t.Fatalf("restored queue was not paused: %+v %v", paused, err)
	}
	if _, _, err := store.ClaimTask(ctx, queued.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("restored item was replayable: %v", err)
	}
	stillRunning, _ := store.GetTask(ctx, running.ID)
	if stillRunning.State != StateRunning {
		t.Fatal("restore invented an outcome for a previously running process")
	}
	events, _ := store.ListEvents(ctx, queued.ID)
	if len(events) != 3 || events[1].Kind != "task.restored_paused" || events[2].Kind != "task.restored_reconcile" {
		t.Fatalf("restore events not idempotent: %+v", events)
	}
	runningEvents, _ := store.ListEvents(ctx, running.ID)
	if len(runningEvents) != 3 || runningEvents[2].Data["publication_requires_readback"] != true {
		t.Fatal("restored running task lost its publication readback requirement")
	}
	if _, err := store.EnqueueResume(ctx, queued.ID, "Reviewed post-backup operations; start this explicit attempt"); err != nil {
		t.Fatal(err)
	}
	// A backup taken after an earlier restore can contain both a historical
	// reconciliation event and a newly queued task. Restoring it must pause the
	// task again; event deduplication must not authorize queue replay.
	if err := store.MarkRestored(ctx); err != nil {
		t.Fatal(err)
	}
	paused, err = store.GetTask(ctx, queued.ID)
	if err != nil || paused.State != StateWaiting {
		t.Fatalf("previous restore event allowed queue replay: %+v %v", paused, err)
	}
	events, err = store.ListEvents(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	reconciliations := 0
	for _, event := range events {
		if event.Kind == "task.restored_reconcile" {
			reconciliations++
		}
	}
	if reconciliations != 1 {
		t.Fatalf("restore reconciliation marker duplicated: %+v", events)
	}
}
