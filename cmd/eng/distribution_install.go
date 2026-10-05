package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/distribution"
)

type installOptions struct{ from, into, source string }

func parseInstall(args []string, verify bool) (installOptions, error) {
	var o installOptions
	fs := flag.NewFlagSet("distribution-install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if verify {
		fs.StringVar(&o.into, "dir", "", "installed directory")
	} else {
		fs.StringVar(&o.from, "from", "", "authenticated extracted distribution")
		fs.StringVar(&o.into, "into", "", "new installation directory")
	}
	fs.StringVar(&o.source, "source-commit", "", "externally expected source commit")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return o, fmt.Errorf("duplicate installation flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 || o.into == "" || o.source == "" || !verify && o.from == "" {
		return o, fmt.Errorf("explicit installation paths and source-commit required")
	}
	return o, nil
}

func distributionInstall(args []string, verify bool) error {
	o, err := parseInstall(args, verify)
	if err != nil {
		return err
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Path != distribution.Module+"/cmd/eng" {
		return fmt.Errorf("source-identified eng executable required")
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["vcs.revision"] != o.source || settings["vcs.modified"] != "false" {
		return fmt.Errorf("installer and expected distribution source differ")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var r distribution.InstallationResult
	if verify {
		r, err = distribution.VerifyInstallation(ctx, o.into, o.source)
	} else {
		r, err = distribution.Install(ctx, o.from, o.into, o.source)
	}
	if err != nil {
		return err
	}
	printJSON(r)
	return nil
}
