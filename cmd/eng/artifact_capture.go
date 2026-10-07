package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

func parseExecutionCapture(args []string) (workeragent.ExecutionCaptureRequest, error) {
	var q workeragent.ExecutionCaptureRequest
	fs := flag.NewFlagSet("artifact-set capture-execution", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&q.Readback.Records, "records", "", "private producer record directory")
	fs.StringVar(&q.Readback.RunID, "run", "", "exact original Run")
	fs.StringVar(&q.Readback.ExecutionID, "execution", "", "exact execution ID")
	fs.StringVar(&q.Readback.PermitDigest, "permit-digest", "", "externally anchored raw permit digest")
	fs.StringVar(&q.Readback.Archive, "archive", "", "explicit referenced source checkpoint")
	fs.StringVar(&q.Readback.Bundle, "bundle", "", "explicit referenced result bundle")
	fs.StringVar(&q.ContextDirectory, "context", "", "original frozen private context directory")
	fs.StringVar(&q.RuntimeBinary, "runtime-binary", "", "private exact Codex runtime binary copy")
	fs.StringVar(&q.QualificationReceipt, "qualification-receipt", "", "private exact Codex qualification receipt")
	fs.StringVar(&q.Destination, "out", "", "new private archive path")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if seen[key] {
				return q, fmt.Errorf("duplicate capture flag")
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return q, fmt.Errorf("invalid capture arguments")
	}
	if err := q.Readback.Validate(); err != nil {
		return q, err
	}
	for _, p := range []string{q.ContextDirectory, q.RuntimeBinary, q.QualificationReceipt, q.Destination} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return q, fmt.Errorf("explicit canonical context/runtime/qualification/output paths required")
		}
	}
	return q, nil
}
func captureExecution(args []string) error {
	q, err := parseExecutionCapture(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r, err := workeragent.CaptureExecutionArtifacts(ctx, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}
