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
	executeCodexMode := flag.Bool("execute-codex", false, "execute one Core-bound WIF Codex engineering turn")
	runID := flag.String("run", "", "exact prepared Run for execution")
	once := flag.Bool("once", false, "perform one cycle then exit")
	flag.Parse()
	modes := 0
	for _, v := range []bool{*admission, *prepare, *execute, *executeCodexMode} {
		if v {
			modes++
		}
	}
	executionMode := *execute || *executeCodexMode
	if modes != 1 || *profile == "" || flag.NArg() != 0 ||
		(executionMode && (!*once || *runID == "")) || (!executionMode && *runID != "") {
		return fmt.Errorf("worker requires exactly one mode and profile; execution modes also require --run <id> --once")
	}
	if os.Getenv("DATABASE_URL") != "" {
		return fmt.Errorf("Worker must not carry DATABASE_URL; use the mTLS Control API")
	}
	config := os.Getenv("WORKER_PREPARATION_CONFIG")
	if *admission && config != "" {
		return fmt.Errorf("admission-only cannot ignore preparation configuration")
	}
	if !*execute && os.Getenv("WORKER_OFFLINE_CONFIG") != "" {
		return fmt.Errorf("non-offline mode cannot ignore WORKER_OFFLINE_CONFIG")
	}
	if !*executeCodexMode && os.Getenv("WORKER_CODEX_CONFIG") != "" {
		return fmt.Errorf("non-Codex mode cannot ignore WORKER_CODEX_CONFIG")
	}
	if *execute && os.Getenv("WORKER_CODEX_CONFIG") != "" {
		return fmt.Errorf("offline execution cannot carry Codex configuration")
	}
	if *executeCodexMode && os.Getenv("WORKER_OFFLINE_CONFIG") != "" {
		return fmt.Errorf("Codex execution cannot carry offline configuration")
	}
	var p *preparation.Preparer
	var err error
	if *prepare || executionMode {
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
	if *executeCodexMode {
		return executeCodex(ctx, client, p, *profile, *runID)
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
