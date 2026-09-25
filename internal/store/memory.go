package store

import (
	"errors"
	"sync"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

var (
	ErrExists   = errors.New("record already exists")
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("optimistic concurrency conflict")
)

type Store interface {
	CreateWork(core.WorkItem) error
	GetWork(string) (core.WorkItem, error)
	UpdateWork(string, uint64, core.WorkItem) error

	GetRecovery() (recovery.Manager, error)
	BeginRecovery(uint64) (recovery.Manager, error)
	CompleteRecovery(uint64, bool) (recovery.Manager, error)

	CreateTaskAndUpdateWork(core.TaskContract, verification.Plan, uint64, core.WorkItem) error
	GetTask(string) (core.TaskContract, error)
	GetTaskRevision(string, uint64) (core.TaskContract, error)
	GetTaskByDigest(string) (core.TaskContract, error)
	GetVerificationPlanByDigest(string) (verification.Plan, error)

	CreateExecutionAndUpdateWork(run.Run, run.Attempt, session.Session, core.RunInputManifest, uint64, core.WorkItem) error
	GetExecution(string) (run.Run, session.Session, error)
	GetAttempt(string, string) (run.Attempt, error)
	GetRunInputByDigest(string) (core.RunInputManifest, error)
	UpdateExecution(string, uint64, run.Run, session.Session) error
	RecordSteering(string, uint64, run.Run, session.Session, session.SteeringCommand) error
	GetSteering(string) (session.SteeringCommand, error)
	UpdateExecutionAndWork(string, uint64, run.Run, session.Session, uint64, core.WorkItem) error
	CreateCheckpoint(session.Checkpoint) (string, error)
	GetCheckpoint(string) (session.Checkpoint, string, error)

	CreateDelivery(core.DeliveryReceipt) error
	GetDelivery(string) (core.DeliveryReceipt, error)

	CreateEvidence(core.EvidenceRef) error
	GetEvidence(string) (core.EvidenceRef, error)

	CreateVerification(verification.Report) error
	GetVerification(string) (verification.Report, error)

	CreateClosureAndUpdateWork(core.ClosureReceipt, uint64, core.WorkItem) error
	GetClosure(string) (core.ClosureReceipt, error)
}

type Memory struct {
	mu                  sync.RWMutex
	recoveryState       recovery.Manager
	works               map[string]core.WorkItem
	tasks               map[string]map[uint64]core.TaskContract
	latestTaskRev       map[string]uint64
	tasksByDigest       map[string]core.TaskContract
	verificationPlans   map[string]verification.Plan
	runs                map[string]run.Run
	attempts            map[string]map[string]run.Attempt
	sessions            map[string]session.Session
	runInputs           map[string]core.RunInputManifest
	steeringCommands    map[string]session.SteeringCommand
	checkpoints         map[string]session.Checkpoint
	checkpointDigests   map[string]string
	operations          map[string]action.Operation
	actionIdempotency   map[string]string
	deliveries          map[string]core.DeliveryReceipt
	evidence            map[string]core.EvidenceRef
	verificationReports map[string]verification.Report
	closures            map[string]core.ClosureReceipt
}

func NewMemory() *Memory {
	return &Memory{
		recoveryState:       *recovery.New(),
		works:               make(map[string]core.WorkItem),
		tasks:               make(map[string]map[uint64]core.TaskContract),
		latestTaskRev:       make(map[string]uint64),
		tasksByDigest:       make(map[string]core.TaskContract),
		verificationPlans:   make(map[string]verification.Plan),
		runs:                make(map[string]run.Run),
		attempts:            make(map[string]map[string]run.Attempt),
		sessions:            make(map[string]session.Session),
		runInputs:           make(map[string]core.RunInputManifest),
		steeringCommands:    make(map[string]session.SteeringCommand),
		checkpoints:         make(map[string]session.Checkpoint),
		checkpointDigests:   make(map[string]string),
		operations:          make(map[string]action.Operation),
		actionIdempotency:   make(map[string]string),
		deliveries:          make(map[string]core.DeliveryReceipt),
		evidence:            make(map[string]core.EvidenceRef),
		verificationReports: make(map[string]verification.Report),
		closures:            make(map[string]core.ClosureReceipt),
	}
}

func (m *Memory) GetRecovery() (recovery.Manager, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recoveryState, nil
}

func (m *Memory) BeginRecovery(expectedEpoch uint64) (recovery.Manager, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryState.Epoch != expectedEpoch {
		return recovery.Manager{}, ErrConflict
	}
	if m.recoveryState.Mode != recovery.Normal {
		return recovery.Manager{}, recovery.ErrAlreadyRecovering
	}
	m.recoveryState.Begin()
	return m.recoveryState, nil
}

func (m *Memory) CompleteRecovery(epoch uint64, reconciled bool) (recovery.Manager, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryState.Epoch != epoch {
		return recovery.Manager{}, ErrConflict
	}
	if err := m.recoveryState.Complete(epoch, reconciled); err != nil {
		return recovery.Manager{}, err
	}
	return m.recoveryState, nil
}

func (m *Memory) CreateWork(item core.WorkItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.works[item.ID]; ok {
		return ErrExists
	}
	if item.Version == 0 {
		item.Version = 1
	}
	m.works[item.ID] = item
	return nil
}

func (m *Memory) GetWork(id string) (core.WorkItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.works[id]
	if !ok {
		return core.WorkItem{}, ErrNotFound
	}
	return item, nil
}

func (m *Memory) UpdateWork(id string, expectedVersion uint64, item core.WorkItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.works[id]
	if !ok {
		return ErrNotFound
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	item.Version = expectedVersion + 1
	m.works[id] = item
	return nil
}

func (m *Memory) CreateTask(task core.TaskContract, plan verification.Plan) error {
	if task.Revision == 0 || task.VerificationPlanDigest == "" || task.VerificationPlanID == "" {
		return ErrConflict
	}
	taskDigest, err := task.Digest()
	if err != nil {
		return err
	}
	planDigest, err := plan.Digest()
	if err != nil {
		return err
	}
	if plan.ID != task.VerificationPlanID || planDigest != task.VerificationPlanDigest {
		return ErrConflict
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tasksByDigest[taskDigest]; ok {
		return ErrExists
	}
	if existing, ok := m.verificationPlans[planDigest]; ok {
		existingDigest, digestErr := existing.Digest()
		if digestErr != nil || existingDigest != planDigest {
			return ErrConflict
		}
	}

	versions := m.tasks[task.ID]
	latest := m.latestTaskRev[task.ID]
	if versions == nil {
		if task.Revision != 1 {
			return ErrConflict
		}
		versions = make(map[uint64]core.TaskContract)
		m.tasks[task.ID] = versions
	} else {
		if _, exists := versions[task.Revision]; exists {
			return ErrExists
		}
		if task.Revision != latest+1 {
			return ErrConflict
		}
	}

	versions[task.Revision] = task
	m.latestTaskRev[task.ID] = task.Revision
	m.tasksByDigest[taskDigest] = task
	m.verificationPlans[planDigest] = plan
	return nil
}

func (m *Memory) CreateTaskAndUpdateWork(task core.TaskContract, plan verification.Plan, expectedWorkVersion uint64, work core.WorkItem) error {
	if task.Revision == 0 || task.VerificationPlanDigest == "" || task.VerificationPlanID == "" {
		return ErrConflict
	}
	taskDigest, err := task.Digest()
	if err != nil {
		return err
	}
	planDigest, err := plan.Digest()
	if err != nil {
		return err
	}
	if plan.ID != task.VerificationPlanID || planDigest != task.VerificationPlanDigest {
		return ErrConflict
	}
	if task.WorkItemID != work.ID || work.ActiveTaskContractDigest != taskDigest || work.State != core.WorkReady {
		return ErrConflict
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	currentWork, ok := m.works[work.ID]
	if !ok {
		return ErrNotFound
	}
	if currentWork.Version != expectedWorkVersion {
		return ErrConflict
	}
	if currentWork.State != core.WorkDraft && currentWork.State != core.WorkReady {
		return ErrConflict
	}
	if currentWork.ActiveRunID != "" {
		return ErrConflict
	}
	if _, ok := m.tasksByDigest[taskDigest]; ok {
		return ErrExists
	}
	if existing, ok := m.verificationPlans[planDigest]; ok {
		existingDigest, digestErr := existing.Digest()
		if digestErr != nil || existingDigest != planDigest {
			return ErrConflict
		}
	}

	versions := m.tasks[task.ID]
	latest := m.latestTaskRev[task.ID]
	if versions == nil {
		if task.Revision != 1 {
			return ErrConflict
		}
		versions = make(map[uint64]core.TaskContract)
	} else {
		if _, exists := versions[task.Revision]; exists {
			return ErrExists
		}
		if task.Revision != latest+1 {
			return ErrConflict
		}
	}

	work.Version = expectedWorkVersion + 1
	m.tasks[task.ID] = versions
	versions[task.Revision] = task
	m.latestTaskRev[task.ID] = task.Revision
	m.tasksByDigest[taskDigest] = task
	m.verificationPlans[planDigest] = plan
	m.works[work.ID] = work
	return nil
}

func (m *Memory) GetTask(id string) (core.TaskContract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	latest, ok := m.latestTaskRev[id]
	if !ok {
		return core.TaskContract{}, ErrNotFound
	}
	return m.tasks[id][latest], nil
}

func (m *Memory) GetTaskRevision(id string, revision uint64) (core.TaskContract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions, ok := m.tasks[id]
	if !ok {
		return core.TaskContract{}, ErrNotFound
	}
	task, ok := versions[revision]
	if !ok {
		return core.TaskContract{}, ErrNotFound
	}
	return task, nil
}

func (m *Memory) GetTaskByDigest(digest string) (core.TaskContract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasksByDigest[digest]
	if !ok {
		return core.TaskContract{}, ErrNotFound
	}
	return task, nil
}

func (m *Memory) GetVerificationPlanByDigest(digest string) (verification.Plan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	plan, ok := m.verificationPlans[digest]
	if !ok {
		return verification.Plan{}, ErrNotFound
	}
	return plan, nil
}

func (m *Memory) CreateExecution(value run.Run, sess session.Session, input core.RunInputManifest) error {
	inputDigest, err := input.Digest()
	if err != nil {
		return err
	}
	if inputDigest != value.RunInputManifestDigest || input.RunID != value.ID || input.TaskContractDigest != value.TaskContractDigest {
		return ErrConflict
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[value.ID]; ok {
		return ErrExists
	}
	if _, ok := m.sessions[value.ID]; ok {
		return ErrExists
	}
	if _, ok := m.runInputs[inputDigest]; ok {
		return ErrExists
	}
	if value.Version == 0 {
		value.Version = 1
	}
	m.runs[value.ID] = value
	m.sessions[value.ID] = sess
	m.runInputs[inputDigest] = input
	return nil
}

func (m *Memory) CreateExecutionAndUpdateWork(value run.Run, attempt run.Attempt, sess session.Session, input core.RunInputManifest, expectedWorkVersion uint64, work core.WorkItem) error {
	inputDigest, err := input.Digest()
	if err != nil {
		return err
	}
	if inputDigest != value.RunInputManifestDigest ||
		input.RunID != value.ID ||
		input.TaskContractDigest != value.TaskContractDigest ||
		attempt.ID != value.CurrentAttemptID ||
		attempt.Epoch != value.CurrentEpoch ||
		sess.RunID != value.ID ||
		sess.ExecutionEpoch != value.CurrentEpoch {
		return ErrConflict
	}
	if work.State != core.WorkExecuting ||
		work.ActiveTaskContractDigest != value.TaskContractDigest ||
		work.ActiveRunID != value.ID {
		return ErrConflict
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	currentWork, ok := m.works[work.ID]
	if !ok {
		return ErrNotFound
	}
	if currentWork.Version != expectedWorkVersion ||
		currentWork.State != core.WorkReady ||
		currentWork.ActiveTaskContractDigest != value.TaskContractDigest ||
		currentWork.ActiveRunID != "" {
		return ErrConflict
	}
	if _, ok := m.tasksByDigest[value.TaskContractDigest]; !ok {
		return ErrNotFound
	}
	if _, ok := m.runs[value.ID]; ok {
		return ErrExists
	}
	if _, ok := m.sessions[value.ID]; ok {
		return ErrExists
	}
	if _, ok := m.runInputs[inputDigest]; ok {
		return ErrExists
	}

	if value.Version == 0 {
		value.Version = 1
	}
	work.Version = expectedWorkVersion + 1
	m.runs[value.ID] = value
	m.sessions[value.ID] = sess
	m.runInputs[inputDigest] = input
	m.attempts[value.ID] = map[string]run.Attempt{attempt.ID: attempt}
	m.works[work.ID] = work
	return nil
}

func (m *Memory) GetAttempt(runID, attemptID string) (run.Attempt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items, ok := m.attempts[runID]
	if !ok {
		return run.Attempt{}, ErrNotFound
	}
	attempt, ok := items[attemptID]
	if !ok {
		return run.Attempt{}, ErrNotFound
	}
	return attempt, nil
}

func (m *Memory) GetExecution(id string) (run.Run, session.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.runs[id]
	if !ok {
		return run.Run{}, session.Session{}, ErrNotFound
	}
	sess, ok := m.sessions[id]
	if !ok {
		return run.Run{}, session.Session{}, ErrNotFound
	}
	return value, sess, nil
}

func (m *Memory) GetRunInputByDigest(digest string) (core.RunInputManifest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	input, ok := m.runInputs[digest]
	if !ok {
		return core.RunInputManifest{}, ErrNotFound
	}
	return input, nil
}

func (m *Memory) CreateCheckpoint(item session.Checkpoint) (string, error) {
	digest, err := item.Digest()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.checkpoints[item.ID]; ok {
		return "", ErrExists
	}
	if _, ok := m.checkpointDigests[digest]; ok {
		return "", ErrExists
	}
	m.checkpoints[item.ID] = item
	m.checkpointDigests[digest] = item.ID
	return digest, nil
}

func (m *Memory) GetCheckpoint(id string) (session.Checkpoint, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.checkpoints[id]
	if !ok {
		return session.Checkpoint{}, "", ErrNotFound
	}
	digest, err := item.Digest()
	if err != nil {
		return session.Checkpoint{}, "", err
	}
	return item, digest, nil
}

func (m *Memory) UpdateExecution(id string, expectedVersion uint64, value run.Run, sess session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runs[id]
	if !ok {
		return ErrNotFound
	}
	if _, ok := m.sessions[id]; !ok {
		return ErrNotFound
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	if current.TaskContractDigest != value.TaskContractDigest ||
		current.RunInputManifestDigest != value.RunInputManifestDigest {
		return ErrConflict
	}
	value.Version = expectedVersion + 1
	m.runs[id] = value
	m.sessions[id] = sess
	return nil
}

func (m *Memory) RecordSteering(id string, expectedVersion uint64, value run.Run, sess session.Session, cmd session.SteeringCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.runs[id]
	if !ok {
		return ErrNotFound
	}
	currentSession, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	if current.TaskContractDigest != value.TaskContractDigest ||
		current.RunInputManifestDigest != value.RunInputManifestDigest ||
		cmd.RunID != id ||
		cmd.ExecutionEpoch != current.CurrentEpoch ||
		sess.ExecutionEpoch != current.CurrentEpoch ||
		sess.LastSequence != cmd.Sequence ||
		cmd.Sequence <= currentSession.LastSequence {
		return ErrConflict
	}
	if _, exists := m.steeringCommands[cmd.ID]; exists {
		return ErrExists
	}

	value.Version = expectedVersion + 1
	m.runs[id] = value
	m.sessions[id] = sess
	m.steeringCommands[cmd.ID] = cmd
	return nil
}

func (m *Memory) GetSteering(id string) (session.SteeringCommand, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cmd, ok := m.steeringCommands[id]
	if !ok {
		return session.SteeringCommand{}, ErrNotFound
	}
	return cmd, nil
}

func (m *Memory) UpdateExecutionAndWork(id string, expectedRunVersion uint64, value run.Run, sess session.Session, expectedWorkVersion uint64, work core.WorkItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentRun, ok := m.runs[id]
	if !ok {
		return ErrNotFound
	}
	if _, ok := m.sessions[id]; !ok {
		return ErrNotFound
	}
	currentWork, ok := m.works[work.ID]
	if !ok {
		return ErrNotFound
	}
	if currentRun.Version != expectedRunVersion || currentWork.Version != expectedWorkVersion {
		return ErrConflict
	}
	if currentRun.TaskContractDigest != value.TaskContractDigest ||
		currentRun.RunInputManifestDigest != value.RunInputManifestDigest {
		return ErrConflict
	}
	if currentWork.ActiveTaskContractDigest != value.TaskContractDigest ||
		currentWork.ActiveRunID != value.ID ||
		work.ActiveTaskContractDigest != value.TaskContractDigest ||
		work.ActiveRunID != value.ID {
		return ErrConflict
	}

	value.Version = expectedRunVersion + 1
	work.Version = expectedWorkVersion + 1
	m.runs[id] = value
	m.sessions[id] = sess
	m.works[work.ID] = work
	return nil
}

func (m *Memory) Create(op action.Operation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.operations[op.ID]; ok {
		return action.ErrOperationExists
	}
	if _, ok := m.actionIdempotency[op.IdempotencyKey]; ok {
		return action.ErrOperationExists
	}
	m.operations[op.ID] = op
	m.actionIdempotency[op.IdempotencyKey] = op.ID
	return nil
}

func (m *Memory) Get(id string) (action.Operation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	op, ok := m.operations[id]
	if !ok {
		return action.Operation{}, action.ErrOperationAbsent
	}
	return op, nil
}

func (m *Memory) GetByIdempotencyKey(key string) (action.Operation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.actionIdempotency[key]
	if !ok {
		return action.Operation{}, action.ErrOperationAbsent
	}
	op, ok := m.operations[id]
	if !ok {
		return action.Operation{}, action.ErrOperationAbsent
	}
	return op, nil
}

func (m *Memory) Update(op action.Operation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.operations[op.ID]; !ok {
		return action.ErrOperationAbsent
	}
	m.operations[op.ID] = op
	return nil
}

func (m *Memory) CreateDelivery(item core.DeliveryReceipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.deliveries[item.ID]; ok {
		return ErrExists
	}
	m.deliveries[item.ID] = item
	return nil
}

func (m *Memory) GetDelivery(id string) (core.DeliveryReceipt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.deliveries[id]
	if !ok {
		return core.DeliveryReceipt{}, ErrNotFound
	}
	return item, nil
}

func (m *Memory) CreateEvidence(item core.EvidenceRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.evidence[item.ID]; ok {
		return ErrExists
	}
	delivery, ok := m.deliveries[item.DeliveryReceiptID]
	if !ok {
		return ErrNotFound
	}
	if item.SubjectDigest != delivery.SubjectDigest {
		return ErrConflict
	}
	task, ok := m.tasksByDigest[delivery.TaskContractDigest]
	if !ok {
		return ErrNotFound
	}
	plan, ok := m.verificationPlans[task.VerificationPlanDigest]
	if !ok {
		return ErrNotFound
	}
	if !verification.EvidenceMatchesPlan(plan, item) || !verification.EvidenceArtifactsBelongToDelivery(delivery, item) {
		return ErrConflict
	}
	m.evidence[item.ID] = item
	return nil
}

func (m *Memory) GetEvidence(id string) (core.EvidenceRef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.evidence[id]
	if !ok {
		return core.EvidenceRef{}, ErrNotFound
	}
	return item, nil
}

func (m *Memory) CreateVerification(report verification.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.verificationReports[report.ID]; ok {
		return ErrExists
	}
	m.verificationReports[report.ID] = report
	return nil
}

func (m *Memory) GetVerification(id string) (verification.Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	report, ok := m.verificationReports[id]
	if !ok {
		return verification.Report{}, ErrNotFound
	}
	return report, nil
}

func (m *Memory) CreateClosureAndUpdateWork(item core.ClosureReceipt, expectedVersion uint64, work core.WorkItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.closures[item.ID]; ok {
		return ErrExists
	}
	current, ok := m.works[work.ID]
	if !ok {
		return ErrNotFound
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	work.Version = expectedVersion + 1
	m.closures[item.ID] = item
	m.works[work.ID] = work
	return nil
}

func (m *Memory) GetClosure(id string) (core.ClosureReceipt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.closures[id]
	if !ok {
		return core.ClosureReceipt{}, ErrNotFound
	}
	return item, nil
}
