package provider

import (
	"context"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type Health struct {
	State       string
	ObservedAt  time.Time
	LastTickAge time.Duration
	Latency     time.Duration
	GapRate     float64
	ErrorRate   float64
	Reconnects  uint64
}

type Subscription interface {
	Close() error
}

type Provider interface {
	Name() string
	Connect(context.Context) error
	Disconnect(context.Context) error
	Subscribe(context.Context, string, chan<- domain.Tick) (Subscription, error)
	History(context.Context, string, string, time.Time, time.Time) ([]domain.Bar, error)
	Health(context.Context) Health
}
