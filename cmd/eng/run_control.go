package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
)

var controlPathID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)

type runControlOptions struct {
	action, runID, textFile string
	input                   codexexec.ControlInput
}

func parseRunControl(args []string) (runControlOptions, error) {
	var o runControlOptions
	if len(args) == 0 {
		return o, fmt.Errorf("usage: eng run-control inspect|status|steer|interrupt <flags>")
	}
	o.action = args[0]
	fs := flag.NewFlagSet("run-control", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.runID, "run", "", "exact Core Run ID")
	fs.StringVar(&o.input.ID, "id", "", "stable command ID (reuse only for readback/exact retry)")
	fs.Uint64Var(&o.input.ExecutionEpoch, "epoch", 0, "current execution epoch")
	fs.Uint64Var(&o.input.Sequence, "sequence", 0, "new increasing Session sequence")
	fs.StringVar(&o.input.ThreadID, "thread", "", "exact active thread ID")
	fs.StringVar(&o.input.TurnID, "turn", "", "exact expected turn ID")
	fs.StringVar(&o.textFile, "text-file", "", "bounded UTF-8 steering input file")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return o, fmt.Errorf("invalid run-control flags")
	}
	switch o.action {
	case "inspect":
		if fs.NFlag() != 1 || !controlPathID.MatchString(o.runID) {
			return o, fmt.Errorf("inspect requires only --run ID")
		}
	case "status":
		if fs.NFlag() != 1 || !controlPathID.MatchString(o.input.ID) {
			return o, fmt.Errorf("status requires only --id ID")
		}
	case "steer", "interrupt":
		if !controlPathID.MatchString(o.runID) || !controlPathID.MatchString(o.input.ID) {
			return o, fmt.Errorf("path-safe Run and command IDs required")
		}
		if o.action == "steer" {
			if o.textFile == "" {
				return o, fmt.Errorf("steer requires --text-file")
			}
			before, err := os.Lstat(o.textFile)
			if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > 8192 {
				return o, fmt.Errorf("bounded regular text file required")
			}
			f, err := os.Open(o.textFile)
			if err != nil {
				return o, err
			}
			defer f.Close()
			opened, err := f.Stat()
			if err != nil || !os.SameFile(before, opened) {
				return o, fmt.Errorf("input changed before read")
			}
			raw, err := io.ReadAll(io.LimitReader(f, 8193))
			if err != nil {
				return o, err
			}
			after, err := os.Lstat(o.textFile)
			if err != nil || !os.SameFile(before, after) || before.Size() != int64(len(raw)) || !before.ModTime().Equal(after.ModTime()) {
				return o, fmt.Errorf("input changed during read")
			}
			o.input.Text = string(raw)
		} else if o.textFile != "" {
			return o, fmt.Errorf("interrupt must not carry text")
		}
		// Validate before reading a TLS identity; the real actor comes solely
		// from the configured client certificate, never a caller-supplied flag.
		test := o.input
		test.Actor = "urn:engineering-platform:validation"
		if err := test.Validate(strings.ToUpper(o.action)); err != nil {
			return o, err
		}
	default:
		return o, fmt.Errorf("unsupported control; pause/resume/checkpoint/takeover are not live controls")
	}
	return o, nil
}
func runControl(args []string) error {
	o, err := parseRunControl(args)
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
	result, err := executeRunControl(ctx, c, o)
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}
func executeRunControl(ctx context.Context, c *controlclient.Client, o runControlOptions) (any, error) {
	if o.action == "inspect" {
		var s codexexec.Status
		err := c.Call(ctx, http.MethodGet, "/api/v1/runs/"+o.runID+"/codex", nil, &s)
		return s, err
	}
	var d codexexec.ControlDelivery
	if o.action == "status" {
		if err := c.Call(ctx, http.MethodGet, "/api/v1/steering/"+o.input.ID+"/delivery", nil, &d); err != nil {
			return nil, err
		}
		if err := d.Validate(); err != nil {
			return nil, err
		}
		if d.Command.ID != o.input.ID {
			return nil, fmt.Errorf("control readback identity mismatch")
		}
		return d, nil
	}
	o.input.Actor = c.Subject()
	if err := o.input.Validate(strings.ToUpper(o.action)); err != nil {
		return nil, err
	}
	// No automatic POST retry and no automatic new ID or sequence. A lost
	// response must be read back by command ID before any manual exact retry.
	if err := c.Call(ctx, http.MethodPost, "/api/v1/runs/"+o.runID+"/"+o.action, o.input, &d); err != nil {
		return nil, err
	}
	if d.Validate() != nil || d.Command.ID != o.input.ID || d.Command.RunID != o.runID || d.Command.Actor != o.input.Actor || d.Command.Sequence != o.input.Sequence || d.Command.ExecutionEpoch != o.input.ExecutionEpoch || d.Payload.Kind != strings.ToUpper(o.action) || d.Payload.Text != o.input.Text || d.Payload.Binding.ThreadID != o.input.ThreadID || d.Payload.Binding.TurnID != o.input.TurnID {
		return nil, fmt.Errorf("control submission/readback mismatch")
	}
	return d, nil
}
