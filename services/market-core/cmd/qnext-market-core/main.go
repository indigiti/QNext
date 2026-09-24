package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

var version = "dev"

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service":  "qnext-market-core",
			"version":  version,
			"built_at": os.Getenv("QNEXT_BUILT_AT"),
		})
	})

	addr := os.Getenv("QNEXT_MARKET_CORE_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("qnext-market-core %s listening on %s", version, addr)
	log.Fatal(server.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
