package runtime

import (
	"context"
	"fmt"
	"os/exec"
)

type LaunchSpec struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	Dir        string   `json:"dir,omitempty"`
	Env        []string `json:"env,omitempty"`
}

type Provider interface {
	Name() string
	Command(context.Context, LaunchSpec) (*exec.Cmd, error)
}

type LocalProcessProvider struct {
	name string
}

func NewLocalProcessProvider(name string) *LocalProcessProvider {
	return &LocalProcessProvider{name: name}
}

func (p *LocalProcessProvider) Name() string {
	return p.name
}

func (p *LocalProcessProvider) Command(ctx context.Context, spec LaunchSpec) (*exec.Cmd, error) {
	if spec.Executable == "" {
		return nil, fmt.Errorf("runtime executable is required")
	}
	cmd := exec.CommandContext(ctx, spec.Executable, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = append(cmd.Environ(), spec.Env...)
	}
	return cmd, nil
}
