package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func main() {
	executable := flag.String("codex", "", "absolute path to the exact native Codex binary")
	version := flag.String("version", codexapp.QualifiedCodexVersion, "expected repository-qualified Codex version")
	model := flag.String("model", "gpt-5.6-sol", "model name used only for local thread/start qualification; no turn is started")
	flag.Parse()
	if *executable == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: codex-qualifier --codex /absolute/native/codex [--version 0.155.0] [--model gpt-5.6-sol]")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	receipt, err := codexapp.Qualify(ctx, *executable, *version, *model)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, err := codexapp.MarshalQualification(receipt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var compact any
	if err := json.Unmarshal(data, &compact); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
