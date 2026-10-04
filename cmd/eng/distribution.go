package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/jiying2007/engineering-platform/internal/distribution"
)

func distributionVerify(args []string) error {
	fs := flag.NewFlagSet("distribution-verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "", "extracted delivery directory")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *dir == "" {
		return fmt.Errorf("usage: eng distribution-verify --dir DIRECTORY")
	}
	result, err := distribution.Verify(*dir)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}
