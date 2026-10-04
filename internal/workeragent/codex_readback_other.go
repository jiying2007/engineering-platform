//go:build !linux

package workeragent

import (
	"fmt"
	"os"
)

func openReadbackFile(root *os.Root, name string) (*os.File, error) {
	return nil, fmt.Errorf("private execution readback requires Linux")
}
