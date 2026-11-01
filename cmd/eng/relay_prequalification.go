package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/jiying2007/engineering-platform/internal/relayqualification"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func relayPrequalification(args []string) error {
	fs := flag.NewFlagSet("relay-prequalification", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contractFile := fs.String("contract", "", "relay provider prequalification contract JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *contractFile == "" {
		return fmt.Errorf("usage: eng relay-prequalification --contract CONTRACT.json")
	}
	contract, err := readRelayContract(*contractFile)
	if err != nil {
		return err
	}
	assessment, err := relayqualification.Evaluate(contract)
	if err != nil {
		return err
	}
	return printJSON(assessment)
}

func readRelayContract(path string) (relayqualification.Contract, error) {
	var contract relayqualification.Contract
	data, err := readRelayInput(path, 64<<10, false)
	if err != nil {
		return contract, fmt.Errorf("bounded regular relay contract file required: %w", err)
	}
	if err := strictjson.Decode(data, &contract); err != nil {
		return relayqualification.Contract{}, fmt.Errorf("strict relay contract: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return relayqualification.Contract{}, err
	}
	return contract, nil
}
