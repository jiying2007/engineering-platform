package main

import (
	"flag"
	"fmt"
	"io"
	"os"

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
	printJSON(assessment)
	return nil
}


func readRelayContract(path string) (relayqualification.Contract, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<10 {
		return relayqualification.Contract{}, fmt.Errorf("bounded regular relay contract file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return relayqualification.Contract{}, err
	}
	var contract relayqualification.Contract
	if err := strictjson.Decode(data, &contract); err != nil {
		return relayqualification.Contract{}, fmt.Errorf("strict relay contract: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return relayqualification.Contract{}, err
	}
	return contract, nil
}
