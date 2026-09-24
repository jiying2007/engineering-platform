package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type registration struct {
	WorkerID          string   `json:"worker_id"`
	RuntimeProviders  []string `json:"runtime_providers"`
	ProtocolVersion   string   `json:"protocol_version"`
	Status            string   `json:"status"`
	RegisteredAt      string   `json:"registered_at"`
}

func main() {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	r := registration{
		WorkerID:         host,
		RuntimeProviders: []string{"codex"},
		ProtocolVersion:  "v1-dev",
		Status:           "READY",
		RegisteredAt:     time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
