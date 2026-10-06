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

type installationSwitchOptions struct {
	active, candidate, previous        string
	activeSource, activeManifest       string
	candidateSource, candidateManifest string
}

func parseInstallationSwitch(args []string) (installationSwitchOptions, error) {
	var out installationSwitchOptions
	fs := flag.NewFlagSet("installation-switch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&out.active, "active", "", "currently active immutable installation")
	fs.StringVar(&out.candidate, "candidate", "", "verified stopped candidate installation")
	fs.StringVar(&out.previous, "previous", "", "new path that will retain the replaced active installation")
	fs.StringVar(&out.activeSource, "active-source", "", "retained active source commit")
	fs.StringVar(&out.activeManifest, "active-manifest-digest", "", "retained active installation manifest digest")
	fs.StringVar(&out.candidateSource, "candidate-source", "", "retained candidate source commit")
	fs.StringVar(&out.candidateManifest, "candidate-manifest-digest", "", "retained candidate installation manifest digest")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return out, fmt.Errorf("duplicate installation switch flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 || out.active == "" || out.candidate == "" || out.previous == "" ||
		out.activeSource == "" || out.activeManifest == "" || out.candidateSource == "" || out.candidateManifest == "" {
		return out, fmt.Errorf("explicit active/candidate/previous paths and retained release identities required")
	}
	return out, nil
}

func installationSwitch(args []string) error {
	options, err := parseInstallationSwitch(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := distribution.SwitchInstalledRelease(
		ctx, options.active, options.candidate, options.previous,
		distribution.ReleaseIdentity{SourceCommit: options.activeSource, ManifestDigest: options.activeManifest},
		distribution.ReleaseIdentity{SourceCommit: options.candidateSource, ManifestDigest: options.candidateManifest},
	)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}
