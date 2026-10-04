package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

type continueOptions struct {
	action, run string
	q           codexexec.ContinueRequest
}

func parseContinue(args []string) (continueOptions, error) {
	var o continueOptions
	if len(args) == 0 {
		return o, fmt.Errorf("run-continue authorize --request FILE | status --run ID")
	}
	o.action = args[0]
	fs := flag.NewFlagSet("run-continue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var file string
	fs.StringVar(&file, "request", "", "private exact decision JSON")
	fs.StringVar(&o.run, "run", "", "source Run ID")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || fs.NFlag() != 1 {
		return o, fmt.Errorf("exactly one operation argument required")
	}
	switch o.action {
	case "status":
		if file != "" || !controlPathID.MatchString(o.run) {
			return o, fmt.Errorf("source Run ID required")
		}
	case "authorize":
		if file == "" || o.run != "" {
			return o, fmt.Errorf("only a private request file is accepted")
		}
		raw, err := readContinueDecision(file)
		if err != nil {
			return o, err
		}
		if len(raw) > 16<<10 || strictjson.Decode(raw, &o.q) != nil || o.q.Validate() != nil || !controlPathID.MatchString(o.q.SourceRunID) || !controlPathID.MatchString(o.q.RunID) {
			return o, fmt.Errorf("invalid continuation decision")
		}
		o.run = o.q.SourceRunID
	default:
		return o, fmt.Errorf("unsupported continuation operation")
	}
	return o, nil
}
func runContinue(args []string) error {
	o, err := parseContinue(args)
	if err != nil {
		return err
	}
	c, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := executeContinue(ctx, c, o)
	if err != nil {
		return err
	}
	printJSON(receipt)
	return nil
}
func executeContinue(ctx context.Context, c *controlclient.Client, o continueOptions) (codexexec.ContinuationReceipt, error) {
	var r codexexec.ContinuationReceipt
	route := "/api/v1/runs/" + o.run
	var err error
	if o.action == "status" {
		err = c.Call(ctx, http.MethodGet, route+"/continuation", nil, &r)
	} else {
		// Exactly one submission. A lost reply must be inspected by the source Run;
		// never allocate a new successor ID or replay a provider operation.
		err = c.Call(ctx, http.MethodPost, route+"/continue", o.q, &r)
	}
	if err != nil {
		return r, err
	}
	if r.Validate() != nil || r.Request.SourceRunID != o.run || (o.action == "authorize" && (r.Actor != c.Subject() || r.Request != o.q)) {
		return r, fmt.Errorf("continuation receipt identity mismatch")
	}
	return r, nil
}

func readContinueDecision(path string) ([]byte, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return nil, fmt.Errorf("aliased decision path")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() <= 0 || before.Size() > 16<<10 {
		return nil, fmt.Errorf("bounded owner-private decision file required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("decision changed before read")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (16<<10)+1))
	if err != nil {
		return nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || int64(len(raw)) != before.Size() {
		return nil, fmt.Errorf("decision changed during read")
	}
	return raw, nil
}
