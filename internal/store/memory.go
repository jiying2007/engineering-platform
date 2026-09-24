package store

import (
	"errors"
	"sync"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

var (
	ErrExists   = errors.New("record already exists")
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("optimistic concurrency conflict")
)

type Store interface {
	CreateWork(core.WorkItem) error
	GetWork(string) (core.WorkItem, error)
	CreateTask(core.TaskContract) error
	GetTask(string) (core.TaskContract, error)
	CreateExecution(run.Run, session.Session) error
	GetExecution(string) (run.Run, session.Session, error)
	UpdateExecution(string, uint64, run.Run, session.Session) error
}

type Memory struct {
	mu       sync.RWMutex
	works    map[string]core.WorkItem
	tasks    map[string]core.TaskContract
	runs     map[string]run.Run
	sessions map[string]session.Session
}

func NewMemory() *Memory {
	return &Memory{
		works:    make(map[string]core.WorkItem),
		tasks:    make(map[string]core.TaskContract),
		runs:     make(map[string]run.Run),
		sessions: make(map[string]session.Session),
	}
}

func (m *Memory) CreateWork(item core.WorkItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.works[item.ID]; ok {
		return ErrExists
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

func (m *Memory) CreateTask(task core.TaskContract) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[task.ID]; ok {
		return ErrExists
	}
	m.tasks[task.ID] = task
	return nil
}

func (m *Memory) GetTask(id string) (core.TaskContract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
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
