package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func relayRuntimeHandoff(args []string) error {
	fs := flag.NewFlagSet("relay-runtime-handoff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contractFile := fs.String("contract", "", "relay provider prequalification v2 contract JSON")
	executable := fs.String("codex", "", "absolute native Codex executable")
	qualificationFile := fs.String("qualification", "", "compatibility qualification receipt JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 ||
		*contractFile == "" || *executable == "" || *qualificationFile == "" {
		return fmt.Errorf("usage: eng relay-runtime-handoff --contract CONTRACT.json --codex ABSOLUTE_CODEX --qualification RECEIPT.json")
	}
	contract, err := readRelayContract(*contractFile)
	if err != nil {
		return err
	}
	qualification, err := readRelayQualification(*qualificationFile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	handoff, err := relayqualification.BuildRuntimeHandoff(ctx, contract, *executable, qualification)
	if err != nil {
		return err
	}
	printJSON(handoff)
	return nil
}

func readRelayQualification(path string) (codexapp.QualificationReceipt, error) {
	var receipt codexapp.QualificationReceipt
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<10 {
		return receipt, fmt.Errorf("bounded regular Codex qualification file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return receipt, err
	}
	if err := strictjson.Decode(data, &receipt); err != nil {
		return codexapp.QualificationReceipt{}, fmt.Errorf("strict Codex qualification: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return codexapp.QualificationReceipt{}, err
	}
	return receipt, nil
}
