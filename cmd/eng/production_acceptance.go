package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

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
	operationalFile := fs.String("operational", "", "version 3 content-addressed operational SLI manifest")
	sourceRoot := fs.String("source-root", "", "read back retained content-addressed collector records")
	runID := fs.String("run", "", "exact measured run identity")
	subject := fs.String("subject", "", "exact measured subject digest")
	deployment := fs.String("deployment", "", "exact measured deployment digest for operational SLI")
	requireVerified := fs.Bool("require-verified", false, "reject a summary without source byte verification")
	requireCriteriaMet := fs.Bool("require-criteria-met", false, "reject operational SLI comparisons that miss explicit criteria; no production qualification")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (*inputFile == "") == (*operationalFile == "") {
		return fmt.Errorf("usage: eng production-slo-report --observations FILE | --operational FILE --source-root DIR --run RUN --subject DIGEST --deployment DIGEST")
	}
	if *operationalFile != "" {
		if *sourceRoot == "" || *runID == "" || *subject == "" || *deployment == "" {
			return fmt.Errorf("operational SLI requires source-root and exact run, subject and deployment")
		}
		data, err := production.ReadOperationalSLOManifest(*operationalFile)
		if err != nil {
			return err
		}
		var input production.OperationalSLOInput
		if err := strictjson.Decode(data, &input); err != nil {
			return fmt.Errorf("strict operational SLI manifest: %w", err)
		}
		envelope, err := production.EvaluateOperationalSLO(input, *sourceRoot, *runID, *subject, *deployment, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := writeOperationalSLOReport(os.Stdout, envelope); err != nil {
			return err
		}
		if *requireCriteriaMet && !envelope.Report.CriteriaMet {
			return fmt.Errorf("operational SLI criteria not met; no qualification is granted")
		}
		return nil
	}
	if *deployment != "" || *requireCriteriaMet {
		return fmt.Errorf("deployment and criteria comparison flags require --operational")
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

func writeOperationalSLOReport(writer io.Writer, envelope production.OperationalSLOEnvelope) error {
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("operational SLI report encoding failed")
	}
	data = append(data, '\n')
	n, err := writer.Write(data)
	if err != nil || n != len(data) {
		return fmt.Errorf("operational SLI report output failed")
	}
	return nil
}
