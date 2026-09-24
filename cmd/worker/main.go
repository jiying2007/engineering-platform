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
	admission := flag.Bool("admission-only", false, "explicitly validate frozen inputs; does NOT run code")
	once := flag.Bool("once", false, "perform one admission cycle then exit")
	flag.Parse()
	if !*admission || *profile == "" || flag.NArg() != 0 {
		return fmt.Errorf("usage: worker --admission-only --profile <profile> [--once]")
	}
	if os.Getenv("DATABASE_URL") != "" {
		return fmt.Errorf("Worker must not carry DATABASE_URL; use the mTLS Control API")
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	step := func(ctx context.Context) error {
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
