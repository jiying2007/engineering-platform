package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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
	report, err := production.ObserveTerminalCore(ctx, client, request)
	if err != nil {
		return err
	}
	// Only a completely checked Core chain emits a report. A successful readback
	// still grants no operational or production qualification.
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("terminal readback report output failed")
	}
	return nil
}
