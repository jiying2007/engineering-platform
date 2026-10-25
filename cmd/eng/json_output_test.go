package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type jsonOutputWriterFunc func([]byte) (int, error)

func (f jsonOutputWriterFunc) Write(data []byte) (int, error) { return f(data) }

type jsonOutputEncodingFailure struct{ err error }

func (v jsonOutputEncodingFailure) MarshalJSON() ([]byte, error) { return nil, v.err }

func TestJSONOutputPreservesEncodedBytes(t *testing.T) {
	value := struct {
		Text   string `json:"text"`
		Ready  bool   `json:"ready"`
		Values []int  `json:"values"`
	}{Text: "<&>\n", Values: []int{2, 1}}
	want := `{
  "text": "\u003c\u0026\u003e\n",
  "ready": false,
  "values": [
    2,
    1
  ]
}
`
	var output bytes.Buffer
	if err := writeJSON(&output, value); err != nil || output.String() != want {
		t.Fatalf("JSON output bytes changed: %q, %v", output.String(), err)
	}
}

func TestJSONOutputRejectsShortWritesAndPreservesErrors(t *testing.T) {
	writeErr := errors.New("output storage unavailable")
	for _, tc := range []struct {
		name  string
		short bool
		err   error
		want  error
	}{
		{"short-without-error", true, nil, io.ErrShortWrite},
		{"partial-with-error", true, writeErr, writeErr},
		{"complete-count-with-error", false, writeErr, writeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			writer := jsonOutputWriterFunc(func(data []byte) (int, error) {
				calls++
				n := len(data)
				if tc.short {
					n--
				}
				return n, tc.err
			})
			if err := writeJSON(writer, map[string]bool{"ready": false}); !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("output failure was lost or retried: %v, writes=%d", err, calls)
			}
		})
	}
}

func TestJSONOutputEncodingFailureWritesNothing(t *testing.T) {
	encodeErr := errors.New("value cannot be encoded")
	for _, tc := range []struct {
		name  string
		value any
		want  error
	}{
		{"unsupported-value", make(chan int), nil},
		{"marshaler-error", jsonOutputEncodingFailure{encodeErr}, encodeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			writer := jsonOutputWriterFunc(func(data []byte) (int, error) {
				calls++
				return len(data), nil
			})
			err := writeJSON(writer, tc.value)
			if err == nil || calls != 0 {
				t.Fatal("encoding failure wrote a partial document or returned success", err, calls)
			}
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatal("encoding error cause was lost", err)
				}
			} else {
				var unsupported *json.UnsupportedTypeError
				if !errors.As(err, &unsupported) {
					t.Fatal("unsupported value error was lost", err)
				}
			}
		})
	}
}

func TestJSONOutputCLIRejectsUnwritableSink(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "eng")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-trimpath", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v %s", err, output)
	}
	manifest := filepath.Join(t.TempDir(), "material.json")
	if err := os.WriteFile(manifest, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var sink *os.File
	var err error
	if runtime.GOOS == "linux" {
		// Exercise real ENOSPC without consuming storage. This is a completed
		// failing write, not a claim that Context interrupts blocked file I/O.
		sink, err = os.OpenFile("/dev/full", os.O_WRONLY, 0)
	} else {
		// A read-only file is a deterministic failing stdout on other hosts.
		sink, err = os.Open(manifest)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	for _, tc := range []struct {
		name         string
		args         []string
		ordinaryExit int
	}{
		{"returning-command", []string{"production-terminal-plan"}, 0},
		{"main-capabilities", []string{"capabilities"}, 0},
		{"main-route", []string{"route", "DEBUG", "Linux UBI storage"}, 0},
		{"main-material-blocked", []string{"material-check", manifest}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ordinary := exec.CommandContext(ctx, binary, tc.args...)
			var output, diagnostics bytes.Buffer
			ordinary.Stdout, ordinary.Stderr = &output, &diagnostics
			err := ordinary.Run()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != tc.ordinaryExit || !json.Valid(output.Bytes()) || !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
				t.Fatalf("ordinary JSON output or existing exit changed: code=%d output=%q stderr=%q", code, output.Bytes(), diagnostics.String())
			}
			failed := exec.CommandContext(ctx, binary, tc.args...)
			diagnostics.Reset()
			failed.Stdout, failed.Stderr = sink, &diagnostics
			err = failed.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(diagnostics.String(), "write JSON output") {
				t.Fatalf("failed JSON output did not reach CLI failure handling: %v stderr=%q", err, diagnostics.String())
			}
		})
	}
}
