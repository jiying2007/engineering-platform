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
	prepare := flag.Bool("prepare-only", false, "prepare operator-approved bytes and independent Git workspace; never execute Runtime")
	once := flag.Bool("once", false, "perform one cycle then exit")
	flag.Parse()
	if *admission == *prepare || *profile == "" || flag.NArg() != 0 {
		return fmt.Errorf("usage: worker (--admission-only | --prepare-only) --profile <profile> [--once]")
	}
	if os.Getenv("DATABASE_URL") != "" {
		return fmt.Errorf("Worker must not carry DATABASE_URL; use the mTLS Control API")
	}
	config := os.Getenv("WORKER_PREPARATION_CONFIG")
	if !*prepare && config != "" {
		return fmt.Errorf("admission-only cannot ignore a supplied preparation configuration")
	}
	var preparer *preparation.Preparer
	var err error
	if *prepare {
		if config == "" {
			return fmt.Errorf("prepare-only requires WORKER_PREPARATION_CONFIG")
		}
		preparer, err = preparation.Load(config)
		if err != nil {
			return err
		}
		defer preparer.Close()
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	if preparer != nil && preparer.Subject() != client.Subject() {
		return fmt.Errorf("preparation policy does not belong to the authenticated Worker")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	step := func(ctx context.Context) error {
		if preparer != nil {
			receipt, err := workeragent.PrepareOnce(ctx, client, *profile, preparer)
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
