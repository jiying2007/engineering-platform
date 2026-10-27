package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
)

func productionTerminalReadback(args []string) error {
	fs := flag.NewFlagSet("production-terminal-readback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	closure := fs.String("closure", "", "exact historical Closure receipt ID")
	runID := fs.String("run", "", "exact historical Run ID")
	subject := fs.String("subject", "", "exact Delivery subject digest")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng production-terminal-readback --closure ID --run ID --subject sha256:...")
	}
	request := production.TerminalReadbackRequest{ClosureID: *closure, RunID: *runID, SubjectDigest: *subject}
	if err := request.Validate(); err != nil {
		return err
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return fmt.Errorf("terminal readback requires configured authenticated Control access")
	}
	defer client.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Include retained Core reads and output in the same bounded invocation.
	ctx, cancel := context.WithTimeout(ctx, production.TerminalReadbackTimeout)
	defer cancel()
	report, err := production.ObserveTerminalCore(ctx, client, request)
	if err != nil {
		return err
	}
	// Only a completely checked Core chain emits a report. A successful readback
	// still grants no operational or production qualification.
	if err := writeProductionDiagnostic(ctx, os.Stdout, report); err != nil {
		return fmt.Errorf("terminal readback report output failed: %w", err)
	}
	return ctx.Err()
}
