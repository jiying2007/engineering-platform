package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func productionTerminalPlan(args []string) error {
	fs := flag.NewFlagSet("production-terminal-plan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng production-terminal-plan")
	}
	envelope, err := production.BuildTerminalPlan()
	if err != nil {
		return err
	}
	printJSON(envelope)
	return nil
}

type productionSLOInput struct {
	Version      int                         `json:"version"`
	Observations []production.SLOObservation `json:"observations"`
}

func productionSLOReport(args []string) error {
	fs := flag.NewFlagSet("production-slo-report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	inputFile := fs.String("observations", "", "measured production SLO observations JSON")
	sourceRoot := fs.String("source-root", "", "read back retained content-addressed collector records")
	runID := fs.String("run", "", "exact measured run identity")
	subject := fs.String("subject", "", "exact measured subject digest")
	requireVerified := fs.Bool("require-verified", false, "reject a summary without source byte verification")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *inputFile == "" {
		return fmt.Errorf("usage: eng production-slo-report --observations FILE")
	}
	info, err := os.Lstat(*inputFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 ||
		info.Size() <= 0 || info.Size() > 1<<20 {
		return fmt.Errorf("bounded non-group/world-writable SLO observation file required")
	}
	data, err := os.ReadFile(*inputFile)
	if err != nil {
		return err
	}
	var input productionSLOInput
	if err := strictjson.Decode(data, &input); err != nil {
		return fmt.Errorf("strict SLO observations: %w", err)
	}
	if input.Version != production.SLOReportVersion {
		return fmt.Errorf("SLO observations require version 2")
	}
	if *sourceRoot == "" && (*runID != "" || *subject != "") {
		return fmt.Errorf("run/subject require source-root")
	}
	var envelope production.SLOReportEnvelope
	if *sourceRoot == "" {
		envelope, err = production.BuildSLOReport(input.Observations)
	} else {
		envelope, err = production.VerifySLOReport(input.Observations, *sourceRoot, *runID, *subject)
	}
	if err != nil {
		return err
	}
	printJSON(envelope)
	if *requireVerified && !envelope.Report.SourceBytesVerified {
		return fmt.Errorf("SLO report is unverified; no qualification is granted")
	}
	return nil
}
