//go:build !linux

package githubpublish

import "os"

func witnessPrivateDirectory(os.FileInfo) bool           { return false }
func witnessPrivateFile(os.FileInfo) bool                { return false }
func openWitnessRead(*os.Root, string) (*os.File, error) { return nil, ErrDispatchEvidenceUnavailable }
