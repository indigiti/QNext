package okx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
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
	HeartbeatInterval time.Duration
	ReadTimeout time.Duration
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

	var repairMu sync.Mutex
	for ctx.Err() == nil {
		reconnecting := attempt > 0
		attempt++
		err := s.once(ctx, contract, func(tradeID string, tick domain.Tick) error {
			if _, exists := seen[tradeID]; exists { return nil }
			seen[tradeID] = struct{}{}
			seenOrder = append(seenOrder, tradeID)
			if len(seenOrder) > 10000 {
				for _, id := range seenOrder[:2000] { delete(seen, id) }
				seenOrder = append([]string(nil), seenOrder[2000:]...)
			}
			sequence++
			tick.Sequence = sequence
			return sink(tick)
		}, func() {
			// Start recovery AFTER the subscribe acknowledgement. Subscribe
			// and process live trades while history reconciliation runs.
			if !reconnecting || s.OnReconnect == nil {return}
			go func() {
				repairMu.Lock()
				defer repairMu.Unlock()
				if ctx.Err()!=nil {return}
				if err:=s.OnReconnect(ctx);err!=nil && ctx.Err()==nil {
					log.Printf("OKX gold reconnect history repair failed: %v",err)
				}
			}()
		})
		if ctx.Err() != nil {return nil}
		log.Printf("OKX gold WebSocket reconnecting: %v", err)
		select {
		case <-ctx.Done(): return nil
		case <-time.After(delay):
		}
	}
	return nil
}

func (s Stream) once(ctx context.Context, contract Contract, sink func(string, domain.Tick) error, onSubscribed func()) error {
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
	// OKX requires clients to send text "ping" after an idle period and
	// replies with text "pong". The sender is independent of the reader.
	heartbeat := s.HeartbeatInterval
	if heartbeat <= 0 {heartbeat = 20*time.Second}
	readTimeout := s.ReadTimeout
	if readTimeout <= 0 {readTimeout = 65*time.Second}
	var lastFrame atomic.Int64
	lastFrame.Store(time.Now().UnixNano())
	var writeMu sync.Mutex
	go func(){
		ticker:=time.NewTicker(heartbeat/2)
		defer ticker.Stop()
		for {
			select {
			case <-closed:return
			case <-ctx.Done():return
			case <-ticker.C:
				if time.Since(time.Unix(0,lastFrame.Load())) < heartbeat {continue}
				writeMu.Lock()
				err:=conn.WriteMessage(websocket.TextMessage,[]byte("ping"))
				writeMu.Unlock()
				if err!=nil {_ = conn.Close();return}
			}
		}
	}()
	subscribed := false
	for {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		lastFrame.Store(time.Now().UnixNano())
		if string(payload) == "pong" {continue}
		if string(payload) == "ping" {
			writeMu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("pong"))
			writeMu.Unlock()
			if err != nil {
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
		if msg.Event == "subscribe" && msg.Arg.InstID==contract.InstID && msg.Arg.Channel=="trades-all" && !subscribed {
			subscribed=true
			if onSubscribed!=nil {onSubscribed()}
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
