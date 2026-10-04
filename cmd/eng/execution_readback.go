package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

type executionReadbackOptions struct {
	request workeragent.PostTurnReadbackRequest
	core    bool
}

func parseExecutionReadback(args []string) (executionReadbackOptions, error) {
	var o executionReadbackOptions
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[name] {
				return o, fmt.Errorf("duplicate execution-readback flag")
			}
			seen[name] = true
		}
	}
	fs := flag.NewFlagSet("execution-readback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.request.Records, "records", "", "absolute private owned record directory")
	fs.StringVar(&o.request.RunID, "run", "", "exact Core Run")
	fs.StringVar(&o.request.ExecutionID, "execution", "", "exact execution ID")
	fs.StringVar(&o.request.PermitDigest, "permit-digest", "", "externally pinned raw permit SHA256")
	fs.StringVar(&o.request.Archive, "archive", "", "explicit source archive for optional byte verification")
	fs.StringVar(&o.request.Bundle, "bundle", "", "explicit result bundle for optional byte verification")
	fs.BoolVar(&o.core, "core", false, "add exactly one authenticated Core status GET")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return o, fmt.Errorf("invalid execution-readback flags")
	}
	return o, o.request.Validate()
}
func executionReadback(args []string) error {
	o, err := parseExecutionReadback(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := workeragent.InspectPostTurn(ctx, o.request)
	if err != nil {
		return err
	}
	if o.core {
		c, err := controlclient.FromEnvironment(os.Getenv)
		if err != nil {
			return err
		}
		defer c.Close()
		result, err = workeragent.ObservePostTurnCore(ctx, c, result)
		if err != nil {
			return err
		}
	}
	printJSON(result)
	return nil
}
