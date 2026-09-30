package main

import "testing"

func TestProductionTerminalPlanRejectsArguments(t *testing.T) {
	if err := productionTerminalPlan([]string{"unexpected"}); err == nil {
		t.Fatal("terminal plan accepted unexpected argument")
	}
}

func TestProductionSLOReportRequiresObservationFile(t *testing.T) {
	if err := productionSLOReport(nil); err == nil {
		t.Fatal("SLO report accepted missing observations")
	}
}
