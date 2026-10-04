//go:build !linux

package processscope

import (
	"context"
	"fmt"
	"os/exec"
)

type Scope struct{}

func Start(*exec.Cmd) (*Scope, error) { return nil, fmt.Errorf("Linux PID namespaces required") }
func (*Scope) Wait(context.Context) (Proof, error) {
	return Proof{}, fmt.Errorf("Linux PID namespaces required")
}
func (*Scope) Stop(context.Context) (Proof, error) {
	return Proof{}, fmt.Errorf("Linux PID namespaces required")
}
