package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/production"
)

type productionObserverCheckOptions struct {
	journal, session, endpoint string
	requireCurrentClear        bool
}

type productionObserverJournalInput interface {
	io.Reader
	Verify(context.Context) error
	Close() error
}

func validProductionObserverSession(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 32 && value == strings.ToLower(value) && err == nil
}

func parseProductionObserverCheck(args []string) (productionObserverCheckOptions, error) {
	var o productionObserverCheckOptions
	fs := flag.NewFlagSet("production-observer-check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.journal, "journal", "", "absolute private observer JSONL journal")
	fs.StringVar(&o.session, "session", "", "externally selected observer session")
	fs.StringVar(&o.endpoint, "endpoint-digest", "", "externally expected configured Control endpoint SHA256")
	fs.BoolVar(&o.requireCurrentClear, "require-current-clear", false, "fail unless the latest observer and complete source window are fresh and clear")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return o, fmt.Errorf("duplicate production-observer-check flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 || !filepath.IsAbs(o.journal) || filepath.Clean(o.journal) != o.journal || strings.ContainsRune(o.journal, 0) || !validProductionObserverSession(o.session) || !strings.HasPrefix(o.endpoint, "sha256:") || len(o.endpoint) != 71 {
		return o, fmt.Errorf("production-observer-check requires --journal, externally selected --session and --endpoint-digest")
	}
	if _, err := hex.DecodeString(o.endpoint[7:]); err != nil || strings.ToLower(o.endpoint) != o.endpoint {
		return o, fmt.Errorf("invalid observer endpoint digest")
	}
	return o, nil
}

func checkProductionObserverJournal(ctx context.Context, o productionObserverCheckOptions, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	journal, err := openProductionObserverJournal(ctx, o.journal)
	if err != nil {
		return err
	}
	defer journal.Close()
	check, err := production.CheckObserverJournal(ctx, journal, o.session, o.endpoint, time.Now)
	if err != nil {
		return err
	}
	if err := journal.Verify(ctx); err != nil {
		return err
	}
	if err := journal.Close(); err != nil {
		return fmt.Errorf("close observer journal: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// File readback may consume time too. Re-evaluate at the completion of the
	// entire read, without accepting a stale result computed before that work.
	check, err = check.Refresh(time.Now())
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeJSON(output, check); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.requireCurrentClear {
		// The printed snapshot keeps its original CheckedAt. A slow sink must
		// not let that earlier freshness grant a successful current-clear exit.
		finished, err := check.Refresh(time.Now())
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !finished.CurrentClear {
			return fmt.Errorf("observer is not currently clear: %s", finished.Reason)
		}
	}
	return nil
}

func productionObserverCheck(args []string) error {
	o, err := parseProductionObserverCheck(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, closeOutput, err := prepareProductionMonitorOutput(ctx, os.Stdout)
	if err != nil {
		return err
	}
	defer closeOutput()
	return checkProductionObserverJournal(ctx, o, output)
}
