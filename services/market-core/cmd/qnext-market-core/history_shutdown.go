package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/history"
)

func init() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := history.CloseAsyncWriters(ctx); err != nil {
			log.Printf("async history shutdown drain: %v", err)
		}
	}()
}
