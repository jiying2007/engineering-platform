package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
)

func productionStatus(args []string) error {
	fs := flag.NewFlagSet("production-status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	requireReady := fs.Bool("require-ready", false, "return nonzero when operational status is not READY")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng production-status [--require-ready]")
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := client.Raw(ctx, http.MethodGet, "/api/v1/operations/status", nil)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var status production.OperationalStatus
	if err := decoder.Decode(&status); err != nil {
		return fmt.Errorf("decode production status: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing production status JSON")
	}
	if _, err := production.EvaluateSnapshot(status.Snapshot); err != nil {
		return err
	}
	printJSON(status)
	if *requireReady && !status.Ready {
		return fmt.Errorf("production operational status is %s", status.State)
	}
	return nil
}
