package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

type offlineConfiguration struct {
	Version int             `json:"version"`
	Socket  string          `json:"engine_socket"`
	Guard   string          `json:"guard_executable"`
	Profile sandbox.Profile `json:"profile"`
}

func executeOffline(ctx context.Context, c *controlclient.Client, p *preparation.Preparer, profile, runID string) error {
	file := os.Getenv("WORKER_OFFLINE_CONFIG")
	if file == "" {
		return fmt.Errorf("WORKER_OFFLINE_CONFIG is required")
	}
	data, err := access.ReadConfiguration(file, false)
	if err != nil {
		return err
	}
	var config offlineConfiguration
	if err = strictjson.Decode(data, &config); err != nil {
		return err
	}
	if config.Version != 1 || config.Profile.Validate() != nil {
		return sandbox.ErrPolicy
	}
	engine, err := sandbox.New(config.Socket, config.Guard)
	if err != nil {
		return err
	}
	defer engine.Close()
	receipt, err := workeragent.ExecuteOffline(ctx, c, p, engine, offline.Start{RunID: runID, WorkerProfile: profile, Profile: config.Profile})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(receipt)
}
