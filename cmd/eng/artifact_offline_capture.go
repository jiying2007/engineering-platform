package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

func parseOfflineCapture(args []string) (workeragent.OfflineCaptureRequest, error) {
	var q workeragent.OfflineCaptureRequest
	fs := flag.NewFlagSet("artifact-set capture-offline", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&q.Records, "records", "", "original private producer record directory")
	fs.StringVar(&q.RunID, "run", "", "exact original Run")
	fs.StringVar(&q.ExecutionID, "execution", "", "exact offline execution ID")
	fs.StringVar(&q.PermitDigest, "permit-digest", "", "externally pinned raw permit digest")
	fs.StringVar(&q.ReportDigest, "report-digest", "", "externally pinned raw local report digest")
	fs.StringVar(&q.Destination, "out", "", "new private archive outside producer records")
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
		return q, fmt.Errorf("invalid offline capture arguments")
	}
	return q, q.Validate()
}

func captureOffline(args []string) error {
	q, err := parseOfflineCapture(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := workeragent.CaptureOfflineBuildArtifacts(ctx, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(r)
}
