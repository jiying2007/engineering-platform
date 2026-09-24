package store

import (
	"errors"
	"sync"

	"github.com/jiying2007/engineering-platform/internal/core"
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

	CreateTask(core.TaskContract) error
	GetTask(string) (core.TaskContract, error)
	GetTaskRevision(string, uint64) (core.TaskContract, error)
	GetTaskByDigest(string) (core.TaskContract, error)

	CreateExecution(run.Run, session.Session) error
	GetExecution(string) (run.Run, session.Session, error)
	UpdateExecution(string, uint64, run.Run, session.Session) error

	CreateDelivery(core.DeliveryReceipt) error
	GetDelivery(string) (core.DeliveryReceipt, error)

	CreateEvidence(core.EvidenceRef) error
	GetEvidence(string) (core.EvidenceRef, error)

	CreateVerification(verification.Report) error
	GetVerification(string) (verification.Report, error)

	CreateClosure(core.ClosureReceipt) error
	GetClosure(string) (core.ClosureReceipt, error)
}

type Memory struct {
	mu                  sync.RWMutex
	works               map[string]core.WorkItem
	tasks               map[string]map[uint64]core.TaskContract
	latestTaskRev       map[string]uint64
	tasksByDigest       map[string]core.TaskContract
	runs                map[string]run.Run
	sessions            map[string]session.Session
	deliveries          map[string]core.DeliveryReceipt
	evidence            map[string]core.EvidenceRef
	verificationReports map[string]verification.Report
	closures            map[string]core.ClosureReceipt
}

func NewMemory() *Memory {
	return &Memory{
		works:               make(map[string]core.WorkItem),
		tasks:               make(map[string]map[uint64]core.TaskContract),
		latestTaskRev:       make(map[string]uint64),
		tasksByDigest:       make(map[string]core.TaskContract),
		runs:                make(map[string]run.Run),
		sessions:            make(map[string]session.Session),
		deliveries:          make(map[string]core.DeliveryReceipt),
		evidence:            make(map[string]core.EvidenceRef),
		verificationReports: make(map[string]verification.Report),
		closures:            make(map[string]core.ClosureReceipt),
	}
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

func (m *Memory) CreateTask(task core.TaskContract) error {
	if task.Revision == 0 {
		return ErrConflict
	}
	digest, err := task.Digest()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tasksByDigest[digest]; ok {
		return ErrExists
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
	m.tasksByDigest[digest] = task
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

func (m *Memory) CreateExecution(value run.Run, sess session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[value.ID]; ok {
		return ErrExists
	}
	if _, ok := m.sessions[value.ID]; ok {
		return ErrExists
	}
	if value.Version == 0 {
		value.Version = 1
	}
	m.runs[value.ID] = value
	m.sessions[value.ID] = sess
	return nil
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
	value.Version = expectedVersion + 1
	m.runs[id] = value
	m.sessions[id] = sess
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

func (m *Memory) CreateClosure(item core.ClosureReceipt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.closures[item.ID]; ok {
		return ErrExists
	}
	m.closures[item.ID] = item
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
