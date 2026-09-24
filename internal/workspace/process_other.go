//go:build !linux

package workspace

import (
	"fmt"
	"os/exec"
)

func configureGitProcess(_ *exec.Cmd) error {
	return fmt.Errorf("independent workspace preparation requires Linux")
}
