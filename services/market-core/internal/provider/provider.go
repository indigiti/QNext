package provider

import (
	"context"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/market"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type Subscription struct {
	InstrumentID string
	ProviderKey  string
	Mode         string
}

type HistoryRequest struct {
	InstrumentID string
	ProviderKey  string
	Timeframe    time.Duration
	From         time.Time
	To           time.Time
}

type Status string

const (
	StatusDisconnected Status = "DISCONNECTED"
	StatusConnecting   Status = "CONNECTING"
	StatusConnected    Status = "CONNECTED"
	StatusDegraded     Status = "DEGRADED"
)

type Health struct {
	Status     Status
	LastTickAt time.Time
	Latency    time.Duration
	ErrorRate  float64
	Reconnects uint64
	ObservedAt time.Time
}

type Adapter interface {
	Name() string
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	SearchSymbols(ctx context.Context, query string) ([]symbol.Instrument, error)
	ResolveSymbol(ctx context.Context, instrumentID string) (symbol.ProviderInstrument, error)
	Subscribe(ctx context.Context, subscriptions []Subscription) error
	Unsubscribe(ctx context.Context, subscriptions []Subscription) error
	History(ctx context.Context, request HistoryRequest) ([]market.Bar, error)
	Status(ctx context.Context) Status
	Health(ctx context.Context) Health
}
