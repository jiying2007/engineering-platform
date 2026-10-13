//go:build !linux

package githubpublish

import "os"

// No uncontained/lock-free fallback for an unsupported host.
func lockWitnessSlot(*os.Root) (*os.File, error) { return nil, ErrDispatchEvidenceUnavailable }

func witnessIncarnation(*os.Root) (string, error) { return "", ErrDispatchEvidenceUnavailable }
