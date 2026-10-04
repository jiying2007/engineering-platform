package main

import (
	"context"
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/processscope"
)

func runtimeIsolationProbe(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: eng runtime-isolation-probe")
	}
	proof, err := processscope.Probe(context.Background())
	if err != nil {
		return err
	}
	printJSON(struct {
		Status              string             `json:"status"`
		Proof               processscope.Proof `json:"proof"`
		ProductionQualified bool               `json:"production_qualified"`
	}{"LOCAL_PROCESS_SCOPE_VERIFIED", proof, false})
	return nil
}
