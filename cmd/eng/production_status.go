package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func productionStatus(args []string) error {
	fs := flag.NewFlagSet("production-status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	requireReady := fs.Bool("require-ready", false, "require observed service readiness, not only database authority health")
	requireAuthority := fs.Bool("require-authority-clear", false, "require only database authority without observed blockers; not service readiness")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (*requireReady && *requireAuthority) {
		return fmt.Errorf("usage: eng production-status [--require-ready | --require-authority-clear]")
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
	status, err := decodeProductionStatus(data, time.Now().UTC())
	if err != nil {
		return err
	}
	printJSON(status)
	if *requireReady && !status.Ready {
		return fmt.Errorf("production operational status is %s", status.State)
	}
	if *requireAuthority && !status.AuthorityClear {
		return fmt.Errorf("database authority status is %s", status.AuthorityState)
	}
	return nil
}
func decodeProductionStatus(data []byte, now time.Time) (production.OperationalStatus, error) {
	var s production.OperationalStatus
	if err := strictjson.Decode(data, &s); err != nil {
		return s, fmt.Errorf("decode production status: %w", err)
	}
	return s, s.ValidateAt(now)
}
