package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/jiying2007/engineering-platform/internal/embedded"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"engineering-control-plane"}`))
	})
	mux.HandleFunc("/api/v1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"capabilities": embedded.Capabilities(),
			"skills":       embedded.Skills(),
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("engineering control plane listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
