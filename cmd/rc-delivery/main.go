package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jiying2007/engineering-platform/internal/cievidence"
	"github.com/jiying2007/engineering-platform/internal/rcdelivery"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func readBounded(path, expectedBase string) ([]byte, error) {
	if path == "" || filepath.Base(path) != expectedBase || filepath.Clean(path) != path {
		return nil, fmt.Errorf("exact RC input path required")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > 1<<20 {
		return nil, fmt.Errorf("bounded regular RC input required")
	}
	return os.ReadFile(path)
}

func main() {
	fs := flag.NewFlagSet("rc-delivery", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ciPath := fs.String("ci-envelope", "", "trusted CI evidence envelope JSON")
	sourceTree := fs.String("source-tree", "", "exact source tree SHA")
	statusPath := fs.String("implementation-status", "", "implementation status markdown")
	evidencePath := fs.String("retained-evidence", "", "retained evidence refs manifest")
	prototypePath := fs.String("retained-prototypes", "", "retained prototype refs manifest")
	if fs.Parse(os.Args[1:]) != nil || fs.NArg() != 0 {
		fail(fmt.Errorf("invalid rc-delivery arguments"))
	}
	ciRaw, err := readBounded(*ciPath, "ci-evidence-envelope.json")
	if err != nil {
		fail(err)
	}
	var ci cievidence.Envelope
	if err := strictjson.Decode(ciRaw, &ci); err != nil {
		fail(err)
	}
	status, err := readBounded(*statusPath, "IMPLEMENTATION_STATUS.md")
	if err != nil {
		fail(err)
	}
	evidence, err := readBounded(*evidencePath, "retained-evidence-refs.json")
	if err != nil {
		fail(err)
	}
	prototypes, err := readBounded(*prototypePath, "retained-prototype-refs.json")
	if err != nil {
		fail(err)
	}
	envelope, err := rcdelivery.Build(ci, *sourceTree, status, evidence, prototypes)
	if err != nil {
		fail(err)
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		fail(err)
	}
	_, err = os.Stdout.Write(append(raw, '\n'))
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
