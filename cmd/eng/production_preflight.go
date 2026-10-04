package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func productionPreflight(args []string) error {
	fs := flag.NewFlagSet("production-preflight", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configFile := fs.String("config", "", "production preflight configuration JSON")
	configOnly := fs.Bool("config-only", false, "validate outer configuration only; never host or live readiness")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *configFile == "" {
		return fmt.Errorf("usage: eng production-preflight --config CONFIG.json")
	}
	info, err := os.Lstat(*configFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() <= 0 || info.Size() > 64<<10 {
		return fmt.Errorf("owner-private bounded regular production config required")
	}
	data, err := os.ReadFile(*configFile)
	if err != nil {
		return err
	}
	var config production.Config
	if err := strictjson.Decode(data, &config); err != nil {
		return fmt.Errorf("strict production config: %w", err)
	}
	var result production.Result
	if *configOnly {
		result, err = production.CheckConfiguration(config)
	} else {
		result, err = production.Check(config)
	}
	if err != nil {
		return err
	}
	printJSON(result)
	return nil
}
