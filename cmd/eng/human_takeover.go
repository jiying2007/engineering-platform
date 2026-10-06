package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

type humanTakeoverOptions struct {
	runID string
	epoch uint64
}

type humanTakeoverReceipt struct {
	Version             int    \`json:"version"\`
	Status              string \`json:"status"\`
	RunID               string \`json:"run_id"\`
	Actor               string \`json:"actor"\`
	PreviousEpoch       uint64 \`json:"previous_epoch"\`
	ExecutionEpoch      uint64 \`json:"execution_epoch"\`
	ControlOwner        string \`json:"control_owner"\`
	RuntimeReplay       bool   \`json:"runtime_replay"\`
	ModelTurnExecuted   bool   \`json:"model_turn_executed"\`
	ProductionQualified bool   \`json:"production_qualified"\`
}

func parseHumanTakeover(args []string) (humanTakeoverOptions, error) {
	var out humanTakeoverOptions
	fs := flag.NewFlagSet("human-takeover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&out.runID, "run", "", "exact Core Run ID")
	fs.Uint64Var(&out.epoch, "epoch", 0, "current execution epoch")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return out, fmt.Errorf("duplicate human-takeover flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 || !controlPathID.MatchString(out.runID) || out.epoch == 0 {
		return out, fmt.Errorf("usage: eng human-takeover --run ID --epoch N")
	}
	return out, nil
}

func executeHumanTakeover(ctx context.Context, c *controlclient.Client, options humanTakeoverOptions) (humanTakeoverReceipt, error) {
	var zero humanTakeoverReceipt
	if c == nil || !controlPathID.MatchString(options.runID) || options.epoch == 0 {
		return zero, fmt.Errorf("authenticated takeover request required")
	}
	var response struct {
		Run     run.Run         \`json:"run"\`
		Session session.Session \`json:"session"\`
	}
	if err := c.Call(ctx, http.MethodPost, "/api/v1/runs/"+options.runID+"/takeover", struct {
		ExecutionEpoch uint64 \`json:"execution_epoch"\`
	}{ExecutionEpoch: options.epoch}, &response); err != nil {
		return zero, err
	}
	if response.Run.ID != options.runID ||
		response.Run.State != run.HumanControlled ||
		response.Run.ControlOwner != "HUMAN" ||
		response.Run.CurrentEpoch != options.epoch+1 ||
		response.Session.RunID != options.runID ||
		response.Session.Owner != session.Human ||
		response.Session.ExecutionEpoch != response.Run.CurrentEpoch {
		return zero, fmt.Errorf("human takeover readback mismatch")
	}
	return humanTakeoverReceipt{
		Version: 1, Status: "HUMAN_CONTROL_ACQUIRED",
		RunID: options.runID, Actor: c.Subject(),
		PreviousEpoch: options.epoch, ExecutionEpoch: response.Run.CurrentEpoch,
		ControlOwner: string(response.Session.Owner),
	}, nil
}

func humanTakeover(args []string) error {
	options, err := parseHumanTakeover(args)
	if err != nil {
		return err
	}
	client, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := executeHumanTakeover(ctx, client, options)
	if err != nil {
		return err
	}
	printJSON(receipt)
	return nil
}
