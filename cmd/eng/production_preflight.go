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
	config, err := loadProductionConfig(*configFile)
	if err != nil {
		return err
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

func loadProductionConfig(path string) (production.Config, error) {
	var config production.Config
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() <= 0 || info.Size() > 64<<10 {
		return config, fmt.Errorf("owner-private bounded regular production config required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	if err := strictjson.Decode(data, &config); err != nil {
		return config, fmt.Errorf("strict production config: %w", err)
	}
	return config, nil
}
