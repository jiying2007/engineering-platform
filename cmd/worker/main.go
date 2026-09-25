package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
	"github.com/jiying2007/engineering-platform/internal/workerloop"
)

func main() {
	if err := serveWorker(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func serveWorker() error {
	profile := flag.String("profile", "", "exact operator-authorized WorkerProfile")
	admission := flag.Bool("admission-only", false, "validate frozen input identities only")
	prepare := flag.Bool("prepare-only", false, "prepare approved bytes without executing Runtime")
	execute := flag.Bool("execute-offline", false, "execute a pinned offline profile for one previously prepared Run")
	runID := flag.String("run", "", "exact prepared Run for offline execution")
	once := flag.Bool("once", false, "perform one cycle then exit")
	flag.Parse()
	modes := 0
	for _, v := range []bool{*admission, *prepare, *execute} {
		if v {
			modes++
		}
	}
	if modes != 1 || *profile == "" || flag.NArg() != 0 || (*execute && (!*once || *runID == "")) || (!*execute && *runID != "") {
		return fmt.Errorf("worker requires exactly one mode and profile; execute-offline also requires --run <id> --once")
	}
	if os.Getenv("DATABASE_URL") != "" {
		return fmt.Errorf("Worker must not carry DATABASE_URL; use the mTLS Control API")
	}
	config := os.Getenv("WORKER_PREPARATION_CONFIG")
	if *admission && config != "" {
		return fmt.Errorf("admission-only cannot ignore preparation configuration")
	}
	if !*execute && os.Getenv("WORKER_OFFLINE_CONFIG") != "" {
		return fmt.Errorf("non-execution mode cannot ignore offline configuration")
	}
	var p *preparation.Preparer
	var err error
	if *prepare || *execute {
		if config == "" {
			return fmt.Errorf("WORKER_PREPARATION_CONFIG required")
		}
		p, err = preparation.Load(config)
		if err != nil {
			return err
		}
		defer p.Close()
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	if p != nil && p.Subject() != client.Subject() {
		return fmt.Errorf("preparation policy does not belong to authenticated Worker")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *execute {
		return executeOffline(ctx, client, p, *profile, *runID)
	}
	step := func(ctx context.Context) error {
		if p != nil {
			receipt, err := workeragent.PrepareOnce(ctx, client, *profile, p)
			if err != nil {
				return err
			}
			if receipt != nil {
				return json.NewEncoder(os.Stdout).Encode(receipt)
			}
			return nil
		}
		receipt, err := workeragent.Once(ctx, client, *profile)
		if err != nil {
			return err
		}
		if receipt != nil {
			return json.NewEncoder(os.Stdout).Encode(receipt)
		}
		return nil
	}
	if *once {
		return step(ctx)
	}
	err = workerloop.Run(ctx, time.Second, func(ctx context.Context) error {
		err := step(ctx)
		var rejected *controlclient.HTTPError
		if errors.As(err, &rejected) && rejected.Status == 409 {
			return nil
		}
		return err
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
