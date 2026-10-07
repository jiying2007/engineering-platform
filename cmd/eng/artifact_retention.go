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

	"github.com/jiying2007/engineering-platform/internal/retention"
)

type artifactRetentionOptions struct {
	config, digest string
}

func parseArtifactRetention(args []string) (artifactRetentionOptions, error) {
	var out artifactRetentionOptions
	if len(args) == 0 || args[0] != "replicate" {
		return out, fmt.Errorf("usage: eng artifact-retention replicate --config FILE --config-digest DIGEST")
	}
	fs := flag.NewFlagSet("artifact-retention replicate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&out.config, "config", "", "externally governed private retention config")
	fs.StringVar(&out.digest, "config-digest", "", "externally pinned raw config SHA256")
	seen := map[string]bool{}
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return out, fmt.Errorf("duplicate artifact-retention flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || out.config == "" || out.digest == "" {
		return out, fmt.Errorf("usage: eng artifact-retention replicate --config FILE --config-digest DIGEST")
	}
	return out, nil
}

func artifactRetention(args []string) error {
	options, err := parseArtifactRetention(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	report, err := retention.Replicate(ctx, options.config, options.digest)
	if err != nil {
		return err
	}
	printJSON(report)
	return nil
}
