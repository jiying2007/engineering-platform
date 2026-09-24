package store

import (
	"errors"
	"sync"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
)

var (
	ErrExists   = errors.New("record already exists")
	ErrNotFound = errors.New("record not found")
)

type Memory struct {
	mu    sync.RWMutex
	works map[string]core.WorkItem
	tasks map[string]core.TaskContract
	runs  map[string]run.Run
}

func NewMemory() *Memory {
	return &Memory{
		works: make(map[string]core.WorkItem),
		tasks: make(map[string]core.TaskContract),
		runs:  make(map[string]run.Run),
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

func (m *Memory) CreateRun(value run.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[value.ID]; ok {
		return ErrExists
	}
	m.runs[value.ID] = value
	return nil
}

func (m *Memory) GetRun(id string) (run.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.runs[id]
	if !ok {
		return run.Run{}, ErrNotFound
	}
	return value, nil
}
