package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestProductionDiagnosticOutputBytesAndNoAuthorityChange(t *testing.T) {
	report := struct {
		State string `json:"state"`
		Ready bool   `json:"ready"`
	}{State: "NOT_OBSERVED"}
	var got bytes.Buffer
	if err := writeProductionDiagnostic(context.Background(), &got, report); err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := writeJSON(&want, report); err != nil {
		t.Fatal(err)
	}
	if got.String() != want.String() {
		t.Fatalf("diagnostic output drift: got=%q want=%q", got.String(), want.String())
	}
}

func TestProductionDiagnosticOutputCancellationIsNotSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	writer := jsonOutputWriterFunc(func(data []byte) (int, error) {
		calls++
		cancel()
		return len(data), nil
	})
	err := writeProductionDiagnostic(ctx, writer, map[string]bool{"ready": false})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled after output incorrectly succeeded: calls=%d err=%v", calls, err)
	}
	// No completed read-only report, no output or late write, when already canceled.
	if err := writeProductionDiagnostic(ctx, writer, true); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled before output attempted a write: calls=%d err=%v", calls, err)
	}
}

func TestProductionDiagnosticOutputRequiresCompleteWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"short", nil},
		{"explicit-error", io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			writer := jsonOutputWriterFunc(func(data []byte) (int, error) {
				calls++
				return len(data) - 1, tc.err
			})
			err := writeProductionDiagnostic(context.Background(), writer, map[string]string{"state": "UNKNOWN"})
			if err == nil || calls != 1 {
				t.Fatalf("short/error write returned success or repeated: %v calls=%d", err, calls)
			}
		})
	}
}
