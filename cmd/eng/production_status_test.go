package main

import (
	"testing"
)

func TestProductionStatusRejectsUnexpectedArguments(t *testing.T) {
	if err := productionStatus([]string{"unexpected"}); err == nil {
		t.Fatal("unexpected production-status argument accepted")
	}
}
