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

	"github.com/jiying2007/engineering-platform/internal/githubpublish"
)

func parseProductionPublisherHealth(args []string) (string, error) {
	fs := flag.NewFlagSet("production-publisher-health", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	config := fs.String("config", "", "owner-controlled publisher remote configuration")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return "", fmt.Errorf("duplicate production-publisher-health flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 || *config == "" {
		return "", fmt.Errorf("usage: eng production-publisher-health --config publisher-remote.json")
	}
	return *config, nil
}

func productionPublisherHealth(args []string) error {
	config, err := parseProductionPublisherHealth(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := githubpublish.LoadRemoteHealth(ctx, config)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}
