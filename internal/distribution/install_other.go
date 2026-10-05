//go:build !linux

package distribution

import (
	"context"
	"fmt"
)

func install(context.Context, string, string, string) (InstallationResult, error) {
	return InstallationResult{}, fmt.Errorf("installation requires Linux")
}
func verifyInstallation(context.Context, string, string) (InstallationResult, error) {
	return InstallationResult{}, fmt.Errorf("installation verification requires Linux")
}
