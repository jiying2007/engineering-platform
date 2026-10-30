//go:build !linux

package main

import (
	"fmt"
	"os"
)

func openRelayRootInput(*os.Root, string) (*os.File, error) {
	return nil, fmt.Errorf("anchored no-follow relay input is unsupported on this platform")
}
