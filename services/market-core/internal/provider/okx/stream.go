package okx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type tradeMessage struct {
	Arg struct {
		Channel string `json:"channel"`
		InstID  string `json:"instId"`
	} `json:"arg"`
	Event string `json:"event"`
	Code  string `json:"code"`
	Msg   string `json:"msg"`
	Data  []struct {
		TradeID string `json:"tradeId"`
		Px      string `json:"px"`
		Sz      string `json:"sz"`
		Ts      string `json:"ts"`
	} `json:"data"`
}

// TradeSink receives normalized executed-trade ticks. Each price and quantity
// comes from an actual execution, not a mark price or forex quote.
type TradeSink func(domain.Tick) error

type Stream struct {
	Endpoint       string
	ReconnectDelay time.Duration
	OnReconnect    func(context.Context) error
}

func (s Stream) endpoint() string {
	if s.Endpoint != "" {
		return s.Endpoint
	}
	return defaultWS
}

// Run holds a subscription with bounded reconnects. Failed authentication
// or malformed provider messages never silently become invented candles.
func (s Stream) Run(ctx context.Context, contract Contract, sink TradeSink) error {
	if contract.InstID == "" || contract.ContractValue <= 0 || sink == nil {
		return errors.New("verified OKX contract and trade sink required")
	}
	delay := s.ReconnectDelay
	if delay <= 0 {
		delay = 3 * time.Second
	}
	seen := make(map[string]struct{}, 12000)
	seenOrder := make([]string, 0, 12000)
	var sequence uint64
	attempt := 0
	for ctx.Err() == nil {
		if attempt > 0 && s.OnReconnect != nil {
			if err := s.OnReconnect(ctx); err != nil && ctx.Err() == nil {
				log.Printf("OKX gold reconnect history repair failed: %v", err)
			}
		}
		attempt++
		err := s.once(ctx, contract, func(tradeID string, tick domain.Tick) error {
			if _, exists := seen[tradeID]; exists {
				return nil
			}
			seen[tradeID] = struct{}{}
			seenOrder = append(seenOrder, tradeID)
			if len(seenOrder) > 10000 {
				for _, id := range seenOrder[:2000] {
					delete(seen, id)
				}
				seenOrder = append([]string(nil), seenOrder[2000:]...)
			}
			sequence++
			tick.Sequence = sequence
			return sink(tick)
		})
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("OKX gold WebSocket reconnecting: %v", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
	return nil
}

func (s Stream) once(ctx context.Context, contract Contract, sink func(string, domain.Tick) error) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, s.endpoint(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{
		"op": "subscribe", "args": []map[string]string{{"channel": "trades-all", "instId": contract.InstID}},
	}); err != nil {
		return err
	}
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(35 * time.Second))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if string(payload) == "ping" {
			if err := conn.WriteMessage(websocket.TextMessage, []byte("pong")); err != nil {
				return err
			}
			continue
		}
		var msg tradeMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return fmt.Errorf("OKX trade message: %w", err)
		}
		if msg.Event == "error" {
			return fmt.Errorf("OKX subscribe: %s %s", msg.Code, msg.Msg)
		}
		if msg.Event != "" {
			continue
		}
		if msg.Arg.InstID != contract.InstID || msg.Arg.Channel != "trades-all" {
			continue
		}
		for _, t := range msg.Data {
			id := strings.TrimSpace(t.TradeID)
			price, err := strconv.ParseFloat(t.Px, 64)
			if err != nil {
				return err
			}
			size, err := strconv.ParseFloat(t.Sz, 64)
			if err != nil {
				return err
			}
			ms, err := strconv.ParseInt(t.Ts, 10, 64)
			if err != nil {
				return err
			}
			if id == "" || price <= 0 || size <= 0 || ms <= 0 {
				return errors.New("invalid OKX trade values")
			}
			now := time.Now().UTC()
			tick := domain.Tick{
				InstrumentID: InstrumentID, Provider: ProviderName,
				Price: price, Quantity: size * contract.ContractValue,
				EventTime: time.UnixMilli(ms).UTC(), TradeTime: time.UnixMilli(ms).UTC(),
				ReceivedTime: now, Quality: domain.QualityGood,
			}
			if err := sink(id, tick); err != nil {
				return err
			}
		}
	}
}
