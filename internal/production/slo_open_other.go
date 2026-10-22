//go:build !linux

package production

import (
	"fmt"
	"os"
)

// Legacy v2 readback is independent of this explicit v3 platform boundary.
func openOperationalSLIFile(_ *os.Root, _ string) (*os.File, error) {
	return nil, fmt.Errorf("operational SLI v3 safe source readback is unsupported on this platform")
}
