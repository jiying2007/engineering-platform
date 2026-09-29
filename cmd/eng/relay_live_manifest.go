package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
)

func relayLiveManifest(args []string) error {
	fs := flag.NewFlagSet("relay-live-manifest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contractFile := fs.String("contract", "", "relay provider prequalification v2 contract JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *contractFile == "" {
		return fmt.Errorf("usage: eng relay-live-manifest --contract CONTRACT.json")
	}
	contract, err := readRelayContract(*contractFile)
	if err != nil {
		return err
	}
	envelope, err := relayqualification.BuildLiveManifest(contract)
	if err != nil {
		return err
	}
	printJSON(envelope)
	return nil
}
