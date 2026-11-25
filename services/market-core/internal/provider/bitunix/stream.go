package bitunix

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type trade struct {
	Price json.RawMessage `json:"p"`
	Size  json.RawMessage `json:"v"`
	Time  json.RawMessage `json:"t"`
}

type message struct {
	Channel string  `json:"ch"`
	Symbol  string  `json:"symbol"`
	Op      string  `json:"op"`
	Code    int     `json:"code"`
	Data    []trade `json:"data"`
}

func parseTrade(c Contract, t trade, now time.Time) (domain.Tick, error) {
	price, err := number(t.Price)
	if err != nil || price <= 0 {
		return domain.Tick{}, errors.New("Bitunix invalid trade price")
	}
	size, err := number(t.Size)
	if err != nil || size <= 0 {
		return domain.Tick{}, errors.New("Bitunix invalid trade amount")
	}
	raw := strings.Trim(string(t.Time), "\"")
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		ms, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || ms < 1000000000000 {
			return domain.Tick{}, errors.New("Bitunix invalid trade timestamp")
		}
		at = time.UnixMilli(ms)
	}
	if at.Before(now.Add(-time.Minute)) || at.After(now.Add(5*time.Second)) {
		return domain.Tick{}, errors.New("Bitunix stale or future trade")
	}
	// Public trades have no stable trade ID or sequence. Preserve identical
	// real trades, but mark evidence degraded; no invented deduplication.
	return domain.Tick{InstrumentID: c.ID(), Provider: ProviderName, Price: price, Quantity: size,
		EventTime: at.UTC(), TradeTime: at.UTC(), ReceivedTime: now.UTC(), Quality: domain.QualityDegraded}, nil
}

type Stream struct {
	Endpoint string
}

// Run uses one bounded reader/heartbeat per opted-in symbol. Coalesce history
// reconciliation instead of accumulating goroutines during reconnect storms.
func (s Stream) Run(ctx context.Context, c Contract, sink func(domain.Tick) error, repair func(context.Context) error) error {
	if !Supported(c.Symbol) || sink == nil {
		return errors.New("Bitunix verified contract and sink required")
	}
	repairs := make(chan struct{}, 1)
	var sequence uint64
	var previous time.Time
	onTrade := func(t domain.Tick) error {
		if t.EventTime.Before(previous) {
			return errors.New("Bitunix out-of-order trade; recovery required")
		}
		previous = t.EventTime
		sequence++
		t.Sequence = sequence // local observation order, not exchange sequence
		return sink(t)
	}
	for ctx.Err() == nil {
		err := s.once(ctx, c, onTrade, func() {
			if repair == nil {
				return
			}
			select {
			case repairs <- struct{}{}:
				go func() {
					defer func() { <-repairs }()
					work, cancel := context.WithTimeout(ctx, 30*time.Second)
					defer cancel()
					if err := repair(work); err != nil && ctx.Err() == nil {
						log.Printf("Bitunix %s history repair incomplete: %v", c.Symbol, err)
					}
				}()
			default:
			}
		})
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("Bitunix %s reconnect: %v", c.Symbol, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
	return nil
}

func (s Stream) once(ctx context.Context, c Contract, sink func(domain.Tick) error, subscribed func()) error {
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = "wss://fapi.bitunix.com/public/"
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(2 << 20)
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(map[string]any{"op": "subscribe", "args": []map[string]string{{"symbol": c.Symbol, "ch": "trade"}}}); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				_ = conn.Close()
				return
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(map[string]any{"op": "ping", "ping": time.Now().Unix()}); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	var first sync.Once
	for {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var m message
		if err := json.Unmarshal(payload, &m); err != nil {
			return err
		}
		if m.Code != 0 || m.Op == "error" {
			return errors.New("Bitunix subscription rejected")
		}
		if m.Channel != "trade" || m.Symbol != c.Symbol || len(m.Data) == 0 {
			continue
		}
		now := time.Now().UTC()
		for _, row := range m.Data {
			tick, err := parseTrade(c, row, now)
			if err != nil {
				return err
			}
			if err := sink(tick); err != nil {
				return err
			}
			first.Do(subscribed)
		}
	}
}
