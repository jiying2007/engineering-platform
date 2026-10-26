//go:build !linux

package production

import (
	"fmt"
	"os"
)

// V2 input and v3 manifests share the hardened file opener; neither can
// safely read untrusted paths on unsupported hosts, so both fail closed.
func openOperationalSLIFile(_ *os.Root, _ string) (*os.File, error) {
	return nil, fmt.Errorf("operational SLI v3 safe source readback is unsupported on this platform")
}
