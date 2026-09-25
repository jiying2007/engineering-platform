package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

var _ corestore.Store = (*Store)(nil)

func bg() context.Context {
	return context.Background()
}

func encodeJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON: %w", err)
	}
	return data, nil
}

func mapReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return corestore.ErrNotFound
	}
	return err
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return corestore.ErrExists
		case "23503", "23514":
			return corestore.ErrConflict
		}
	}
	return err
}

func auditInput(eventType, aggregateType, aggregateID string, payload any) (audit.Input, error) {
	digest, err := canonical.Digest(payload)
	if err != nil {
		return audit.Input{}, err
	}
	return audit.Input{
		Type:          eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		PayloadDigest: digest,
	}, nil
}

func (s *Store) CreateWork(item core.WorkItem) error {
	if item.Version == 0 {
		item.Version = 1
	}
	input, err := auditInput("work.created", "WorkItem", item.ID, item)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
INSERT INTO work_items (
    work_item_id,source_ref,title,human_owner,target_id,assurance_class,
    active_task_contract_digest,active_run_id,state,version,created_at,updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())`
			_, err := tx.Exec(
				ctx, q,
				item.ID, nullIfEmpty(item.SourceRef), item.Title, item.HumanOwner,
				nullIfEmpty(item.TargetID), nullIfEmpty(item.AssuranceClass),
				nullIfEmpty(item.ActiveTaskContractDigest), nullIfEmpty(item.ActiveRunID),
				string(item.State), item.Version, item.CreatedAt,
			)
			return mapWriteError(err)
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetWork(id string) (core.WorkItem, error) {
	const q = `
SELECT work_item_id,COALESCE(source_ref,''),title,human_owner,COALESCE(target_id,''),
       COALESCE(assurance_class,''),COALESCE(active_task_contract_digest,''),
       COALESCE(active_run_id,''),state,version,created_at
FROM work_items
WHERE work_item_id=$1`
	var item core.WorkItem
	var state string
	err := s.pool.QueryRow(bg(), q, id).Scan(
		&item.ID, &item.SourceRef, &item.Title, &item.HumanOwner, &item.TargetID,
		&item.AssuranceClass, &item.ActiveTaskContractDigest, &item.ActiveRunID,
		&state, &item.Version, &item.CreatedAt,
	)
	if err != nil {
		return core.WorkItem{}, mapReadError(err)
	}
	item.State = core.WorkState(state)
	return item, nil
}

func (s *Store) UpdateWork(id string, expectedVersion uint64, item core.WorkItem) error {
	payload := struct {
		ExpectedVersion uint64        `json:"expected_version"`
		Work            core.WorkItem `json:"work"`
	}{ExpectedVersion: expectedVersion, Work: item}
	input, err := auditInput("work.updated", "WorkItem", id, payload)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
UPDATE work_items
SET source_ref=$1,title=$2,human_owner=$3,target_id=$4,assurance_class=$5,
    active_task_contract_digest=$6,active_run_id=$7,state=$8,version=version+1,updated_at=now()
WHERE work_item_id=$9 AND version=$10`
			tag, err := tx.Exec(
				ctx, q,
				nullIfEmpty(item.SourceRef), item.Title, item.HumanOwner,
				nullIfEmpty(item.TargetID), nullIfEmpty(item.AssuranceClass),
				nullIfEmpty(item.ActiveTaskContractDigest), nullIfEmpty(item.ActiveRunID),
				string(item.State), id, expectedVersion,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) CreateTaskAndUpdateWork(
	task core.TaskContract,
	plan verification.Plan,
	expectedWorkVersion uint64,
	work core.WorkItem,
) error {
	taskDigest, err := task.Digest()
	if err != nil {
		return err
	}
	planDigest, err := plan.Digest()
	if err != nil {
		return err
	}
	if task.VerificationPlanID != plan.ID ||
		task.VerificationPlanDigest != planDigest ||
		task.WorkItemID != work.ID ||
		work.ActiveTaskContractDigest != taskDigest ||
		work.State != core.WorkReady {
		return corestore.ErrConflict
	}
	taskJSON, err := encodeJSON(task)
	if err != nil {
		return err
	}
	planJSON, err := encodeJSON(plan)
	if err != nil {
		return err
	}
	payload := struct {
		Task core.TaskContract `json:"task"`
		Plan verification.Plan `json:"verification_plan"`
		Work core.WorkItem     `json:"work"`
	}{Task: task, Plan: plan, Work: work}
	input, err := auditInput("task.frozen", "TaskContract", task.ID, payload)
	if err != nil {
		return err
	}

	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			var currentVersion uint64
			var currentState string
			var currentRun string
			const lockWork = `
SELECT version,state,COALESCE(active_run_id,'')
FROM work_items
WHERE work_item_id=$1
FOR UPDATE`
			if err := tx.QueryRow(ctx, lockWork, work.ID).Scan(&currentVersion, &currentState, &currentRun); err != nil {
				return mapReadError(err)
			}
			if currentVersion != expectedWorkVersion ||
				(currentState != string(core.WorkDraft) && currentState != string(core.WorkReady)) ||
				currentRun != "" {
				return corestore.ErrConflict
			}

			var latest uint64
			if err := tx.QueryRow(
				ctx,
				"SELECT COALESCE(MAX(revision),0) FROM task_contracts WHERE task_contract_id=$1",
				task.ID,
			).Scan(&latest); err != nil {
				return err
			}
			if task.Revision != latest+1 {
				return corestore.ErrConflict
			}

			const insertPlan = `
INSERT INTO verification_plans (verification_plan_id,plan_digest,plan_json)
VALUES ($1,$2,$3::jsonb)
ON CONFLICT (plan_digest) DO NOTHING`
			if _, err := tx.Exec(ctx, insertPlan, plan.ID, planDigest, string(planJSON)); err != nil {
				return mapWriteError(err)
			}

			const insertTask = `
INSERT INTO task_contracts (
    task_contract_id,revision,work_item_id,task_type,content_digest,repository,base_commit,
    target_id,verification_plan_id,verification_plan_digest,contract_json
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)`
			if _, err := tx.Exec(
				ctx, insertTask,
				task.ID, task.Revision, task.WorkItemID, task.TaskType, taskDigest,
				task.Repository, task.BaseCommit, nullIfEmpty(task.TargetID),
				task.VerificationPlanID, task.VerificationPlanDigest, string(taskJSON),
			); err != nil {
				return mapWriteError(err)
			}

			const updateWork = `
UPDATE work_items
SET active_task_contract_digest=$1,active_run_id=$2,state=$3,version=version+1,updated_at=now()
WHERE work_item_id=$4 AND version=$5`
			tag, err := tx.Exec(
				ctx, updateWork,
				nullIfEmpty(work.ActiveTaskContractDigest), nullIfEmpty(work.ActiveRunID),
				string(work.State), work.ID, expectedWorkVersion,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetTask(id string) (core.TaskContract, error) {
	const q = `
SELECT contract_json
FROM task_contracts
WHERE task_contract_id=$1
ORDER BY revision DESC
LIMIT 1`
	return s.readTask(q, id)
}

func (s *Store) GetTaskRevision(id string, revision uint64) (core.TaskContract, error) {
	const q = "SELECT contract_json FROM task_contracts WHERE task_contract_id=$1 AND revision=$2"
	return s.readTask(q, id, revision)
}

func (s *Store) GetTaskByDigest(digest string) (core.TaskContract, error) {
	const q = "SELECT contract_json FROM task_contracts WHERE content_digest=$1"
	return s.readTask(q, digest)
}

func (s *Store) readTask(query string, args ...any) (core.TaskContract, error) {
	var raw []byte
	if err := s.pool.QueryRow(bg(), query, args...).Scan(&raw); err != nil {
		return core.TaskContract{}, mapReadError(err)
	}
	var task core.TaskContract
	if err := json.Unmarshal(raw, &task); err != nil {
		return core.TaskContract{}, fmt.Errorf("decode task contract: %w", err)
	}
	return task, nil
}

func (s *Store) GetVerificationPlanByDigest(digest string) (verification.Plan, error) {
	var raw []byte
	if err := s.pool.QueryRow(
		bg(),
		"SELECT plan_json FROM verification_plans WHERE plan_digest=$1",
		digest,
	).Scan(&raw); err != nil {
		return verification.Plan{}, mapReadError(err)
	}
	var plan verification.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return verification.Plan{}, fmt.Errorf("decode verification plan: %w", err)
	}
	return plan, nil
}

func (s *Store) CreateExecutionAndUpdateWork(
	value run.Run,
	attempt run.Attempt,
	sess session.Session,
	inputManifest core.RunInputManifest,
	expectedWorkVersion uint64,
	work core.WorkItem,
) error {
	inputDigest, err := inputManifest.Digest()
	if err != nil {
		return err
	}
	if inputDigest != value.RunInputManifestDigest ||
		inputManifest.RunID != value.ID ||
		inputManifest.TaskContractDigest != value.TaskContractDigest ||
		attempt.ID != value.CurrentAttemptID ||
		attempt.Epoch != value.CurrentEpoch ||
		sess.RunID != value.ID ||
		sess.ExecutionEpoch != value.CurrentEpoch ||
		work.State != core.WorkExecuting ||
		work.ActiveTaskContractDigest != value.TaskContractDigest ||
		work.ActiveRunID != value.ID {
		return corestore.ErrConflict
	}
	manifestJSON, err := encodeJSON(inputManifest)
	if err != nil {
		return err
	}
	payload := struct {
		Run     run.Run               `json:"run"`
		Attempt run.Attempt           `json:"attempt"`
		Session session.Session       `json:"session"`
		Input   core.RunInputManifest `json:"run_input"`
		Work    core.WorkItem         `json:"work"`
	}{Run: value, Attempt: attempt, Session: sess, Input: inputManifest, Work: work}
	auditRecord, err := auditInput("run.started", "Run", value.ID, payload)
	if err != nil {
		return err
	}

	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			var currentVersion uint64
			var currentState string
			var currentTask string
			var currentRun string
			const lockWork = `
SELECT version,state,COALESCE(active_task_contract_digest,''),COALESCE(active_run_id,'')
FROM work_items
WHERE work_item_id=$1
FOR UPDATE`
			if err := tx.QueryRow(ctx, lockWork, work.ID).Scan(
				&currentVersion, &currentState, &currentTask, &currentRun,
			); err != nil {
				return mapReadError(err)
			}
			if currentVersion != expectedWorkVersion ||
				currentState != string(core.WorkReady) ||
				currentTask != value.TaskContractDigest ||
				currentRun != "" {
				return corestore.ErrConflict
			}

			var exists bool
			if err := tx.QueryRow(
				ctx,
				"SELECT EXISTS(SELECT 1 FROM task_contracts WHERE content_digest=$1)",
				value.TaskContractDigest,
			).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return corestore.ErrNotFound
			}

			const insertInput = `
INSERT INTO run_input_manifests (
    run_input_manifest_digest,run_id,task_contract_digest,runtime_profile,
    tool_profile,worker_profile,policy_profile,manifest_json
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb)`
			if _, err := tx.Exec(
				ctx, insertInput,
				inputDigest, inputManifest.RunID, inputManifest.TaskContractDigest,
				inputManifest.RuntimeProfile, inputManifest.ToolProfile,
				inputManifest.WorkerProfile, inputManifest.PolicyProfile,
				string(manifestJSON),
			); err != nil {
				return mapWriteError(err)
			}

			const insertRun = `
INSERT INTO runs (
    run_id,task_contract_digest,run_input_manifest_digest,state,version,current_epoch,
    current_attempt_id,control_owner
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
			if _, err := tx.Exec(
				ctx, insertRun,
				value.ID, value.TaskContractDigest, value.RunInputManifestDigest,
				string(value.State), value.Version, value.CurrentEpoch,
				nullIfEmpty(value.CurrentAttemptID), value.ControlOwner,
			); err != nil {
				return mapWriteError(err)
			}

			const insertAttempt = `
INSERT INTO run_attempts (run_id,attempt_id,execution_epoch,worker_id,disposition,started_at,ended_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)`
			if _, err := tx.Exec(
				ctx, insertAttempt,
				value.ID, attempt.ID, attempt.Epoch, nil, nil, attempt.StartedAt, nil,
			); err != nil {
				return mapWriteError(err)
			}

			const insertSession = `
INSERT INTO sessions (run_id,execution_epoch,control_owner,last_steering_sequence,paused)
VALUES ($1,$2,$3,$4,$5)`
			if _, err := tx.Exec(
				ctx, insertSession,
				sess.RunID, sess.ExecutionEpoch, string(sess.Owner), sess.LastSequence, sess.Paused,
			); err != nil {
				return mapWriteError(err)
			}

			const updateWork = `
UPDATE work_items
SET active_task_contract_digest=$1,active_run_id=$2,state=$3,version=version+1,updated_at=now()
WHERE work_item_id=$4 AND version=$5`
			tag, err := tx.Exec(
				ctx, updateWork,
				work.ActiveTaskContractDigest, work.ActiveRunID, string(work.State),
				work.ID, expectedWorkVersion,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: auditRecord,
		Outbox: []OutboxMessage{{
			Key:           "run:" + value.ID + ":started",
			Topic:         "run.started",
			AggregateType: "Run",
			AggregateID:   value.ID,
			Payload: map[string]any{
				"run_id":                    value.ID,
				"task_contract_digest":      value.TaskContractDigest,
				"run_input_manifest_digest": value.RunInputManifestDigest,
				"execution_epoch":           value.CurrentEpoch,
			},
		}},
	})
	return err
}

func (s *Store) GetExecution(id string) (run.Run, session.Session, error) {
	const q = `
SELECT r.run_id,r.task_contract_digest,r.run_input_manifest_digest,r.state,r.version,
       r.current_epoch,COALESCE(r.current_attempt_id,''),r.control_owner,
       s.execution_epoch,s.control_owner,s.last_steering_sequence,s.paused
FROM runs r
JOIN sessions s ON s.run_id=r.run_id
WHERE r.run_id=$1`
	var value run.Run
	var sess session.Session
	var runState string
	var sessionOwner string
	if err := s.pool.QueryRow(bg(), q, id).Scan(
		&value.ID, &value.TaskContractDigest, &value.RunInputManifestDigest,
		&runState, &value.Version, &value.CurrentEpoch, &value.CurrentAttemptID,
		&value.ControlOwner, &sess.ExecutionEpoch, &sessionOwner,
		&sess.LastSequence, &sess.Paused,
	); err != nil {
		return run.Run{}, session.Session{}, mapReadError(err)
	}
	value.State = run.State(runState)
	sess.RunID = value.ID
	sess.Owner = session.ControlOwner(sessionOwner)
	return value, sess, nil
}

func (s *Store) GetAttempt(runID, attemptID string) (run.Attempt, error) {
	const q = `
SELECT attempt_id,execution_epoch,started_at,ended_at
FROM run_attempts
WHERE run_id=$1 AND attempt_id=$2`
	var item run.Attempt
	var endedAt pgtype.Timestamptz
	if err := s.pool.QueryRow(bg(), q, runID, attemptID).Scan(
		&item.ID, &item.Epoch, &item.StartedAt, &endedAt,
	); err != nil {
		return run.Attempt{}, mapReadError(err)
	}
	if endedAt.Valid {
		item.EndedAt = endedAt.Time
	}
	return item, nil
}

func (s *Store) GetRunInputByDigest(digest string) (core.RunInputManifest, error) {
	var raw []byte
	if err := s.pool.QueryRow(
		bg(),
		"SELECT manifest_json FROM run_input_manifests WHERE run_input_manifest_digest=$1",
		digest,
	).Scan(&raw); err != nil {
		return core.RunInputManifest{}, mapReadError(err)
	}
	var item core.RunInputManifest
	if err := json.Unmarshal(raw, &item); err != nil {
		return core.RunInputManifest{}, fmt.Errorf("decode run input manifest: %w", err)
	}
	return item, nil
}

func (s *Store) UpdateExecution(id string, expectedVersion uint64, value run.Run, sess session.Session) error {
	payload := struct {
		ExpectedVersion uint64          `json:"expected_version"`
		Run             run.Run         `json:"run"`
		Session         session.Session `json:"session"`
	}{ExpectedVersion: expectedVersion, Run: value, Session: sess}
	input, err := auditInput("run.updated", "Run", id, payload)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const updateRun = `
UPDATE runs
SET state=$1,version=version+1,current_epoch=$2,current_attempt_id=$3,control_owner=$4,updated_at=now()
WHERE run_id=$5 AND version=$6
  AND task_contract_digest=$7
  AND run_input_manifest_digest=$8`
			tag, err := tx.Exec(
				ctx, updateRun,
				string(value.State), value.CurrentEpoch, nullIfEmpty(value.CurrentAttemptID),
				value.ControlOwner, id, expectedVersion,
				value.TaskContractDigest, value.RunInputManifestDigest,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}

			const updateSession = `
UPDATE sessions
SET execution_epoch=$1,control_owner=$2,last_steering_sequence=$3,paused=$4,updated_at=now()
WHERE run_id=$5`
			tag, err = tx.Exec(
				ctx, updateSession,
				sess.ExecutionEpoch, string(sess.Owner), sess.LastSequence, sess.Paused, id,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) RecordSteering(
	id string,
	expectedVersion uint64,
	value run.Run,
	sess session.Session,
	cmd session.SteeringCommand,
) error {
	payload := struct {
		Run      run.Run                 `json:"run"`
		Session  session.Session         `json:"session"`
		Steering session.SteeringCommand `json:"steering"`
	}{Run: value, Session: sess, Steering: cmd}
	input, err := auditInput("steering.recorded", "Run", id, payload)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			var currentEpoch uint64
			var currentTask, currentInput string
			if err := tx.QueryRow(
				ctx,
				"SELECT current_epoch,task_contract_digest,run_input_manifest_digest FROM runs WHERE run_id=$1 FOR UPDATE",
				id,
			).Scan(&currentEpoch, &currentTask, &currentInput); err != nil {
				return mapReadError(err)
			}
			if currentEpoch != cmd.ExecutionEpoch ||
				currentTask != value.TaskContractDigest ||
				currentInput != value.RunInputManifestDigest ||
				sess.ExecutionEpoch != currentEpoch ||
				sess.LastSequence != cmd.Sequence {
				return corestore.ErrConflict
			}

			const insertSteer = `
INSERT INTO steering_commands (
    steering_command_id,run_id,execution_epoch,sequence,actor,content_digest,delivery_state,created_at
) VALUES ($1,$2,$3,$4,$5,$6,'RECORDED',$7)`
			if _, err := tx.Exec(
				ctx, insertSteer,
				cmd.ID, cmd.RunID, cmd.ExecutionEpoch, cmd.Sequence,
				cmd.Actor, cmd.ContentDigest, cmd.CreatedAt,
			); err != nil {
				return mapWriteError(err)
			}

			const updateSession = `
UPDATE sessions
SET last_steering_sequence=$1,updated_at=now()
WHERE run_id=$2 AND execution_epoch=$3 AND last_steering_sequence < $1`
			tag, err := tx.Exec(ctx, updateSession, cmd.Sequence, id, cmd.ExecutionEpoch)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}

			const updateRun = `
UPDATE runs
SET version=version+1,updated_at=now()
WHERE run_id=$1 AND version=$2`
			tag, err = tx.Exec(ctx, updateRun, id, expectedVersion)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetSteering(id string) (session.SteeringCommand, error) {
	const q = `
SELECT steering_command_id,run_id,execution_epoch,sequence,actor,content_digest,created_at
FROM steering_commands
WHERE steering_command_id=$1`
	var item session.SteeringCommand
	if err := s.pool.QueryRow(bg(), q, id).Scan(
		&item.ID, &item.RunID, &item.ExecutionEpoch, &item.Sequence,
		&item.Actor, &item.ContentDigest, &item.CreatedAt,
	); err != nil {
		return session.SteeringCommand{}, mapReadError(err)
	}
	return item, nil
}

func (s *Store) UpdateExecutionAndWork(
	id string,
	expectedRunVersion uint64,
	value run.Run,
	sess session.Session,
	expectedWorkVersion uint64,
	work core.WorkItem,
) error {
	payload := struct {
		Run  run.Run         `json:"run"`
		Work core.WorkItem   `json:"work"`
		Sess session.Session `json:"session"`
	}{Run: value, Work: work, Sess: sess}
	input, err := auditInput("run.work.updated", "Run", id, payload)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const updateRun = `
UPDATE runs
SET state=$1,version=version+1,current_epoch=$2,current_attempt_id=$3,control_owner=$4,updated_at=now()
WHERE run_id=$5 AND version=$6
  AND task_contract_digest=$7
  AND run_input_manifest_digest=$8`
			tag, err := tx.Exec(
				ctx, updateRun,
				string(value.State), value.CurrentEpoch, nullIfEmpty(value.CurrentAttemptID),
				value.ControlOwner, id, expectedRunVersion,
				value.TaskContractDigest, value.RunInputManifestDigest,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}

			const updateSession = `
UPDATE sessions
SET execution_epoch=$1,control_owner=$2,last_steering_sequence=$3,paused=$4,updated_at=now()
WHERE run_id=$5`
			tag, err = tx.Exec(
				ctx, updateSession,
				sess.ExecutionEpoch, string(sess.Owner), sess.LastSequence, sess.Paused, id,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}

			const updateWork = `
UPDATE work_items
SET active_task_contract_digest=$1,active_run_id=$2,state=$3,version=version+1,updated_at=now()
WHERE work_item_id=$4 AND version=$5`
			tag, err = tx.Exec(
				ctx, updateWork,
				nullIfEmpty(work.ActiveTaskContractDigest), nullIfEmpty(work.ActiveRunID),
				string(work.State), work.ID, expectedWorkVersion,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) CreateCheckpoint(item session.Checkpoint) (string, error) {
	digest, err := item.Digest()
	if err != nil {
		return "", err
	}
	raw, err := encodeJSON(item)
	if err != nil {
		return "", err
	}
	input, err := auditInput("checkpoint.created", "Run", item.RunID, item)
	if err != nil {
		return "", err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
INSERT INTO checkpoints (
    checkpoint_id,run_id,task_contract_digest,run_input_manifest_digest,execution_epoch,
    source_tree_digest,diff_digest,checkpoint_json,content_digest,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)`
			_, err := tx.Exec(
				ctx, q,
				item.ID, item.RunID, item.TaskContractDigest, item.RunInputManifestDigest,
				item.ExecutionEpoch, item.SourceTreeDigest, nullIfEmpty(item.DiffDigest),
				string(raw), digest, item.CreatedAt,
			)
			return mapWriteError(err)
		},
		Audit: input,
	})
	if err != nil {
		return "", err
	}
	return digest, nil
}

func (s *Store) GetCheckpoint(id string) (session.Checkpoint, string, error) {
	var raw []byte
	var digest string
	if err := s.pool.QueryRow(
		bg(),
		"SELECT checkpoint_json,content_digest FROM checkpoints WHERE checkpoint_id=$1",
		id,
	).Scan(&raw, &digest); err != nil {
		return session.Checkpoint{}, "", mapReadError(err)
	}
	var item session.Checkpoint
	if err := json.Unmarshal(raw, &item); err != nil {
		return session.Checkpoint{}, "", fmt.Errorf("decode checkpoint: %w", err)
	}
	return item, digest, nil
}

func (s *Store) CreateDelivery(item core.DeliveryReceipt) error {
	raw, err := encodeJSON(item)
	if err != nil {
		return err
	}
	input, err := auditInput("delivery.created", "DeliveryReceipt", item.ID, item)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
INSERT INTO delivery_receipts (
    delivery_receipt_id,work_item_id,task_contract_digest,run_id,target_id,
    base_commit,result_commit,subject_digest,receipt_json,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`
			_, err := tx.Exec(
				ctx, q,
				item.ID, item.WorkItemID, item.TaskContractDigest, item.RunID,
				nullIfEmpty(item.TargetID), item.BaseCommit, nullIfEmpty(item.ResultCommit),
				item.SubjectDigest, string(raw), item.CreatedAt,
			)
			return mapWriteError(err)
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetDelivery(id string) (core.DeliveryReceipt, error) {
	var raw []byte
	if err := s.pool.QueryRow(
		bg(),
		"SELECT receipt_json FROM delivery_receipts WHERE delivery_receipt_id=$1",
		id,
	).Scan(&raw); err != nil {
		return core.DeliveryReceipt{}, mapReadError(err)
	}
	var item core.DeliveryReceipt
	if err := json.Unmarshal(raw, &item); err != nil {
		return core.DeliveryReceipt{}, fmt.Errorf("decode delivery receipt: %w", err)
	}
	return item, nil
}

func (s *Store) CreateEvidence(item core.EvidenceRef) error {
	delivery, err := s.GetDelivery(item.DeliveryReceiptID)
	if err != nil {
		return err
	}
	if item.SubjectDigest != delivery.SubjectDigest {
		return corestore.ErrConflict
	}
	task, err := s.GetTaskByDigest(delivery.TaskContractDigest)
	if err != nil {
		return err
	}
	plan, err := s.GetVerificationPlanByDigest(task.VerificationPlanDigest)
	if err != nil {
		return err
	}
	if !verification.EvidenceMatchesPlan(plan, item) || !verification.EvidenceArtifactsBelongToDelivery(delivery, item) {
		return corestore.ErrConflict
	}
	raw, err := encodeJSON(item)
	if err != nil {
		return err
	}
	input, err := auditInput("evidence.registered", "Evidence", item.ID, item)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
INSERT INTO evidence (
    evidence_id,delivery_receipt_id,subject_digest,issuer,procedure_ref,result,applicable,evidence_json
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb)`
			_, err := tx.Exec(
				ctx, q,
				item.ID, item.DeliveryReceiptID, item.SubjectDigest, item.Issuer,
				item.Procedure, item.Result, item.Applicable, string(raw),
			)
			return mapWriteError(err)
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetEvidence(id string) (core.EvidenceRef, error) {
	var raw []byte
	if err := s.pool.QueryRow(
		bg(),
		"SELECT evidence_json FROM evidence WHERE evidence_id=$1",
		id,
	).Scan(&raw); err != nil {
		return core.EvidenceRef{}, mapReadError(err)
	}
	var item core.EvidenceRef
	if err := json.Unmarshal(raw, &item); err != nil {
		return core.EvidenceRef{}, fmt.Errorf("decode evidence: %w", err)
	}
	return item, nil
}

func (s *Store) CreateVerification(report verification.Report) error {
	raw, err := encodeJSON(report)
	if err != nil {
		return err
	}
	input, err := auditInput("verification.created", "VerificationReport", report.ID, report)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
INSERT INTO verification_reports (
    verification_report_id,delivery_receipt_id,verification_plan_id,verification_plan_digest,
    subject_digest,result,verifier,report_json,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)`
			_, err := tx.Exec(
				ctx, q,
				report.ID, report.DeliveryReceiptID, report.PlanID, report.VerificationPlanDigest,
				report.SubjectDigest, report.Result, nullIfEmpty(report.Verifier),
				string(raw), report.CreatedAt,
			)
			return mapWriteError(err)
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetVerification(id string) (verification.Report, error) {
	var raw []byte
	if err := s.pool.QueryRow(
		bg(),
		"SELECT report_json FROM verification_reports WHERE verification_report_id=$1",
		id,
	).Scan(&raw); err != nil {
		return verification.Report{}, mapReadError(err)
	}
	var report verification.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return verification.Report{}, fmt.Errorf("decode verification report: %w", err)
	}
	return report, nil
}

func (s *Store) CreateClosureAndUpdateWork(
	item core.ClosureReceipt,
	expectedVersion uint64,
	work core.WorkItem,
) error {
	raw, err := encodeJSON(item)
	if err != nil {
		return err
	}
	payload := struct {
		Closure core.ClosureReceipt `json:"closure"`
		Work    core.WorkItem       `json:"work"`
	}{Closure: item, Work: work}
	input, err := auditInput("work.closed", "WorkItem", work.ID, payload)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const insertClosure = `
INSERT INTO closure_receipts (
    closure_receipt_id,work_item_id,task_contract_digest,run_id,delivery_receipt_id,
    verification_report_id,review_report_id,subject_digest,result,receipt_json,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`
			if _, err := tx.Exec(
				ctx, insertClosure,
				item.ID, item.WorkItemID, item.TaskContractDigest, item.RunID,
				item.DeliveryReceiptID, item.VerificationReportID,
				nullIfEmpty(item.ReviewReportID), item.SubjectDigest, item.Result,
				string(raw), item.CreatedAt,
			); err != nil {
				return mapWriteError(err)
			}

			const updateWork = `
UPDATE work_items
SET active_task_contract_digest=$1,active_run_id=$2,state=$3,version=version+1,updated_at=now()
WHERE work_item_id=$4 AND version=$5`
			tag, err := tx.Exec(
				ctx, updateWork,
				nullIfEmpty(work.ActiveTaskContractDigest), nullIfEmpty(work.ActiveRunID),
				string(work.State), work.ID, expectedVersion,
			)
			if err != nil {
				return mapWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				return corestore.ErrConflict
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) GetClosure(id string) (core.ClosureReceipt, error) {
	var raw []byte
	if err := s.pool.QueryRow(
		bg(),
		"SELECT receipt_json FROM closure_receipts WHERE closure_receipt_id=$1",
		id,
	).Scan(&raw); err != nil {
		return core.ClosureReceipt{}, mapReadError(err)
	}
	var item core.ClosureReceipt
	if err := json.Unmarshal(raw, &item); err != nil {
		return core.ClosureReceipt{}, fmt.Errorf("decode closure receipt: %w", err)
	}
	return item, nil
}
