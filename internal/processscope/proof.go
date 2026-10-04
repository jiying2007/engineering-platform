// Package processscope binds process-tree termination to a kernel lifecycle,
// not to a protocol ACK, PID list, process group, or model assertion.
package processscope

import (
	"fmt"
	"time"
)

const Mechanism = "linux-user-pid-namespace-init-v1"

// Proof is a trusted Worker observation of a dedicated namespace init. It is
// embedded in the exact execution's sealed transcript; it is not a portable
// credential, checkpoint, or permission to resume or take over a workspace.
type Proof struct {
	Version           int       `json:"version"`
	Mechanism         string    `json:"mechanism"`
	HostPID           int       `json:"host_pid"`
	NamespaceID       uint64    `json:"pid_namespace_inode"`
	ParentNamespaceID uint64    `json:"parent_pid_namespace_inode"`
	StartedAt         time.Time `json:"started_at"`
	ReapedAt          time.Time `json:"reaped_at"`
	InitReaped        bool      `json:"namespace_init_reaped"`
}

func (p Proof) Validate() error {
	if p.Version != 1 || p.Mechanism != Mechanism || p.HostPID <= 0 || p.NamespaceID == 0 || p.ParentNamespaceID == 0 || p.NamespaceID == p.ParentNamespaceID || p.StartedAt.IsZero() {
		return fmt.Errorf("dedicated PID namespace identity required")
	}
	if p.InitReaped {
		if p.ReapedAt.IsZero() || p.ReapedAt.Before(p.StartedAt) {
			return fmt.Errorf("invalid namespace reap observation")
		}
	} else if !p.ReapedAt.IsZero() {
		return fmt.Errorf("unconfirmed namespace carries reap time")
	}
	return nil
}

func (p Proof) Quiescent() bool { return p.Validate() == nil && p.InitReaped }
