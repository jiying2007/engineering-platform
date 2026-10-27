package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type productionStatusOptions struct {
	requireReady, requireAuthority, requireNoAlert bool
	observe                                        bool
	policy                                         production.ProgressPolicy
	monitor                                        bool
	monitorPolicy                                  production.ObserverPolicy
	session                                        string
}

func parseProductionStatus(args []string) (productionStatusOptions, error) {
	var o productionStatusOptions
	fs := flag.NewFlagSet("production-status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.requireReady, "require-ready", false, "require observed service readiness, not only database authority health")
	fs.BoolVar(&o.requireAuthority, "require-authority-clear", false, "require only database authority without observed blockers; not service readiness")
	fs.BoolVar(&o.requireNoAlert, "require-no-alert", false, "require no derived alert from a complete sampled window; not readiness")
	window := fs.Duration("observe-for", 0, "bounded observation window (2s..5m)")
	interval := fs.Duration("interval", 5*time.Second, "fixed sampling interval (1s..60s)")
	maxAge := fs.Duration("max-pending-age", 0, "explicit diagnostic pending-age threshold (1s..24h), not a production SLO")
	monitorFor := fs.Duration("monitor-for", 0, "finite observer budget (complete window cycle..24h), with JSONL diagnostic events")
	staleAfter := fs.Duration("stale-after", 0, "explicit observer/source freshness threshold (window cycle..24h)")
	fs.StringVar(&o.session, "session", "", "externally selected fresh observer session, only with --monitor-for")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return o, fmt.Errorf("duplicate production-status flag")
			}
			seen[key] = true
		}
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (o.requireReady && o.requireAuthority) {
		return o, fmt.Errorf("invalid production-status arguments")
	}
	o.observe = seen["observe-for"]
	o.monitor = seen["monitor-for"]
	if !o.observe {
		if seen["interval"] || seen["max-pending-age"] || seen["require-no-alert"] || o.monitor || seen["stale-after"] || seen["session"] {
			return o, fmt.Errorf("observation options require --observe-for")
		}
		return o, nil
	}
	if seen["require-ready"] || seen["require-authority-clear"] || !seen["max-pending-age"] {
		return o, fmt.Errorf("window requires explicit --max-pending-age and cannot grant snapshot readiness")
	}
	for _, d := range []time.Duration{*window, *interval, *maxAge} {
		if d <= 0 || d%time.Second != 0 {
			return o, fmt.Errorf("whole positive seconds required")
		}
	}
	o.policy = production.ProgressPolicy{WindowSeconds: int(*window / time.Second), IntervalSeconds: int(*interval / time.Second), MaxPendingAgeSeconds: int(*maxAge / time.Second)}
	if err := o.policy.Validate(); err != nil {
		return o, err
	}
	if !o.monitor {
		if seen["stale-after"] || seen["session"] {
			return o, fmt.Errorf("--stale-after and --session require --monitor-for")
		}
		return o, nil
	}
	if seen["session"] && !validProductionObserverSession(o.session) {
		return o, fmt.Errorf("observer session requires 32 lowercase hexadecimal characters")
	}
	if !seen["stale-after"] || *monitorFor <= 0 || *staleAfter <= 0 || *monitorFor%time.Second != 0 || *staleAfter%time.Second != 0 {
		return o, fmt.Errorf("monitor requires explicit whole positive duration and stale threshold")
	}
	o.monitorPolicy = production.ObserverPolicy{Progress: o.policy, DurationSeconds: int(*monitorFor / time.Second), StaleAfterSeconds: int(*staleAfter / time.Second)}
	return o, o.monitorPolicy.Validate()
}

func productionStatus(args []string) error {
	o, err := parseProductionStatus(args)
	if err != nil {
		return err
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if o.monitor {
		hadAlert, err := runProductionMonitor(ctx, client, o.monitorPolicy, os.Getenv("CONTROL_ENDPOINT"), o.session, os.Stdout)
		if err != nil {
			return err
		}
		if o.requireNoAlert && hadAlert {
			return fmt.Errorf("observer recorded a failed or alerted window")
		}
		return nil
	}
	if o.observe {
		// Keep the complete sampling window plus its final JSON output inside
		// an outer bounded lifetime; the inner sampling policy remains intact.
		ctx, cancel := context.WithTimeout(ctx, time.Duration(o.policy.WindowSeconds)*time.Second+15*time.Second)
		defer cancel()
		report, err := collectProductionProgress(ctx, client, o.policy)
		if err != nil {
			return err
		}
		if err := writeProductionDiagnostic(ctx, os.Stdout, report); err != nil {
			return err
		}
		if o.requireNoAlert && report.DiagnosticAlert {
			return fmt.Errorf("operational diagnostic: %s", report.State)
		}
		return ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := client.Raw(ctx, http.MethodGet, "/api/v1/operations/status", nil)
	if err != nil {
		return err
	}
	status, err := decodeProductionStatus(data, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := writeProductionDiagnostic(ctx, os.Stdout, status); err != nil {
		return err
	}
	if err := status.ValidateAt(time.Now().UTC()); err != nil {
		return err
	}
	if o.requireReady && !status.Ready {
		return fmt.Errorf("production operational status is %s", status.State)
	}
	if o.requireAuthority && !status.AuthorityClear {
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
