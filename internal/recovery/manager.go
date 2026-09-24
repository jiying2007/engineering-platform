package recovery

import "errors"

var (
	ErrStaleEpoch             = errors.New("stale recovery epoch")
	ErrRecoveryMode           = errors.New("mutating external actions are blocked during recovery reconciliation")
	ErrAlreadyRecovering      = errors.New("recovery reconciliation is already active")
	ErrReconciliationRequired = errors.New("reconciliation must be confirmed before leaving recovery mode")
)

type Mode string

const (
	Normal                 Mode = "NORMAL"
	RecoveryReconciliation Mode = "RECOVERY_RECONCILIATION"
)

type Manager struct {
	Epoch uint64 `json:"recovery_epoch"`
	Mode  Mode   `json:"mode"`
}

func New() *Manager {
	return &Manager{Mode: Normal}
}

func (m *Manager) Begin() uint64 {
	m.Epoch++
	m.Mode = RecoveryReconciliation
	return m.Epoch
}

func (m *Manager) CheckEpoch(epoch uint64) error {
	if epoch != m.Epoch {
		return ErrStaleEpoch
	}
	return nil
}

func (m *Manager) AuthorizeIrreversible(epoch uint64) error {
	if err := m.CheckEpoch(epoch); err != nil {
		return err
	}
	if m.Mode == RecoveryReconciliation {
		return ErrRecoveryMode
	}
	return nil
}

func (m *Manager) Complete(epoch uint64, reconciled bool) error {
	if err := m.CheckEpoch(epoch); err != nil {
		return err
	}
	if !reconciled {
		return ErrReconciliationRequired
	}
	m.Mode = Normal
	return nil
}
