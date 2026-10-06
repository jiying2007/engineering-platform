package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/distribution"
)

type releaseReadbackOptions struct {
	dir, source, manifest string
}

func parseInstallationReadback(args []string) (releaseReadbackOptions, error) {
	var out releaseReadbackOptions
	fs := flag.NewFlagSet("installation-readback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&out.dir, "dir", "", "installed immutable release directory")
	fs.StringVar(&out.source, "source-commit", "", "externally retained source commit")
	fs.StringVar(&out.manifest, "manifest-digest", "", "externally retained installation manifest digest")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return out, fmt.Errorf("duplicate release readback flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 || out.dir == "" || out.source == "" || out.manifest == "" {
		return out, fmt.Errorf("usage: eng installation-readback --dir DIR --source-commit SHA --manifest-digest sha256:DIGEST")
	}
	return out, nil
}

func installationReadback(args []string) error {
	options, err := parseInstallationReadback(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := distribution.VerifyInstalledRelease(ctx, options.dir, options.source, options.manifest)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}
