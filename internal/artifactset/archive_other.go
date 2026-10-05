//go:build !linux

package artifactset

import (
	"context"
	"fmt"
)

func Pack(context.Context, string, string, string) (Report, error) {
	return Report{}, fmt.Errorf("private artifact sets require Linux")
}
func Verify(context.Context, string, string, string) (Report, error) {
	return Report{}, fmt.Errorf("private artifact sets require Linux")
}
func Restore(context.Context, string, string, string, string) (Report, error) {
	return Report{}, fmt.Errorf("private artifact sets require Linux")
}
