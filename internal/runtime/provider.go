package runtime

import (
	"context"
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
