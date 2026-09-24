package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/history"
	"github.com/indigiti/QNext/services/market-core/internal/httpapi"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	addr := env("QNEXT_HTTP_ADDR", ":8080")
	storageRoot := env("QNEXT_STORAGE_ROOT", "./storage")
	started := time.Now().UTC()

	store := history.New(storageRoot)
	handler := httpapi.New(store, httpapi.Options{
		Version:   version,
		Commit:    commit,
		StartedAt: started,
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("qnext-market-core listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
