package stream

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type WebSocketHandler struct {
	Broker        *Broker
	AllowedOrigin func(*http.Request) bool
}

type clientMessage struct {
	Op        string  `json:"op"`
	Channel   string  `json:"channel,omitempty"`
	Symbol    string  `json:"symbol,omitempty"`
	Timeframe string  `json:"timeframe,omitempty"`
	StreamID  string  `json:"stream_id,omitempty"`
	AfterSeq  *uint64 `json:"after_seq,omitempty"`
}

type serverMessage struct {
	Op        string   `json:"op"`
	Protocol  string   `json:"protocol,omitempty"`
	Channel   string   `json:"channel,omitempty"`
	StreamID  string   `json:"stream_id,omitempty"`
	Seq       uint64   `json:"seq,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	Timeframe string   `json:"timeframe,omitempty"`
	Quality   string   `json:"quality,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Code      string   `json:"code,omitempty"`
	Message   string   `json:"message,omitempty"`
	Bar       *wireBar `json:"bar,omitempty"`
}

type wireBar struct {
	Time     int64   `json:"time"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
	Final    bool    `json:"final"`
	Revision uint32  `json:"revision"`
}

func NewWebSocketHandler(broker *Broker) http.Handler {
	return &WebSocketHandler{Broker: broker}
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Broker == nil {
		http.Error(w, "stream broker unavailable", http.StatusServiceUnavailable)
		return
	}

	checkOrigin := h.AllowedOrigin
	if checkOrigin == nil {
		checkOrigin = sameOrigin
	}
	upgrader := websocket.Upgrader{CheckOrigin: checkOrigin}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	var writeMu sync.Mutex
	write := func(message serverMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return connection.WriteJSON(message)
	}

	if err := write(serverMessage{Op: "hello", Protocol: "QNEXT.STREAM/1"}); err != nil {
		return
	}

	subscriptions := make(map[string]*Subscription)
	defer func() {
		for _, subscription := range subscriptions {
			subscription.Cancel()
		}
	}()

	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() {
		doneOnce.Do(func() { close(done) })
	}
	defer stop()

	for {
		var request clientMessage
		if err := connection.ReadJSON(&request); err != nil {
			return
		}

		switch request.Op {
		case "subscribe":
			if request.Channel != "bars" || request.Symbol == "" || request.Timeframe == "" {
				_ = write(protocolError("INVALID_SUBSCRIBE", "bars subscription requires symbol and timeframe"))
				continue
			}
			subscription, err := h.Broker.Subscribe(request.Symbol, request.Timeframe, nil)
			if err != nil {
				_ = write(protocolError("SUBSCRIBE_FAILED", err.Error()))
				continue
			}
			replaceSubscription(subscriptions, subscription)
			if err := write(serverMessage{
				Op:        "subscribed",
				Channel:   "bars",
				StreamID:  subscription.StreamID,
				Seq:       subscription.CurrentSeq,
				Symbol:    request.Symbol,
				Timeframe: request.Timeframe,
			}); err != nil {
				return
			}
			go forwardSubscription(done, subscription, write)

		case "resume":
			if request.StreamID == "" || request.AfterSeq == nil {
				_ = write(protocolError("INVALID_RESUME", "resume requires stream_id and after_seq"))
				continue
			}
			subscription, err := h.Broker.SubscribeByID(request.StreamID, *request.AfterSeq)
			if err != nil {
				_ = write(protocolError("RESUME_FAILED", err.Error()))
				continue
			}
			if subscription.ResyncRequired {
				_ = write(serverMessage{
					Op:       "resync_required",
					StreamID: request.StreamID,
					Reason:   "replay_unavailable",
				})
				continue
			}
			replaceSubscription(subscriptions, subscription)
			if err := write(serverMessage{
				Op:       "subscribed",
				Channel:  "bars",
				StreamID: subscription.StreamID,
				Seq:      subscription.CurrentSeq,
			}); err != nil {
				return
			}
			for _, event := range subscription.Replay {
				if err := write(updateMessage(event)); err != nil {
					return
				}
			}
			go forwardSubscription(done, subscription, write)

		case "unsubscribe":
			subscription := subscriptions[request.StreamID]
			if subscription != nil {
				subscription.Cancel()
				delete(subscriptions, request.StreamID)
			}

		case "heartbeat":
			if err := write(serverMessage{Op: "heartbeat"}); err != nil {
				return
			}

		default:
			_ = write(protocolError("UNKNOWN_OPERATION", "unsupported stream operation"))
		}
	}
}

func replaceSubscription(subscriptions map[string]*Subscription, subscription *Subscription) {
	if previous := subscriptions[subscription.StreamID]; previous != nil {
		previous.Cancel()
	}
	subscriptions[subscription.StreamID] = subscription
}

func forwardSubscription(
	done <-chan struct{},
	subscription *Subscription,
	write func(serverMessage) error,
) {
	for {
		select {
		case <-done:
			return
		case event, ok := <-subscription.Events:
			if !ok {
				return
			}
			if event.ResyncRequired {
				if err := write(serverMessage{
					Op:       "resync_required",
					StreamID: event.StreamID,
					Reason:   event.Reason,
				}); err != nil {
					return
				}
				continue
			}
			if err := write(updateMessage(event)); err != nil {
				return
			}
		}
	}
}

func updateMessage(event BarEvent) serverMessage {
	return serverMessage{
		Op:        "update",
		Channel:   "bars",
		StreamID:  event.StreamID,
		Seq:       event.Seq,
		Symbol:    event.Bar.InstrumentID,
		Timeframe: event.Bar.Timeframe,
		Quality:   string(event.Bar.Quality),
		Bar:       toWireBar(event.Bar),
	}
}

func toWireBar(bar domain.Bar) *wireBar {
	return &wireBar{
		Time:     bar.OpenTime.UTC().UnixMilli(),
		Open:     bar.Open,
		High:     bar.High,
		Low:      bar.Low,
		Close:    bar.Close,
		Volume:   bar.Volume,
		Final:    bar.Final,
		Revision: bar.Revision,
	}
}

func protocolError(code, message string) serverMessage {
	return serverMessage{
		Op:      "error",
		Code:    code,
		Message: message,
	}
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	for _, host := range requestHosts(r) {
		if strings.EqualFold(parsed.Host, host) {
			return true
		}
	}
	return false
}

func requestHosts(r *http.Request) []string {
	hosts := make([]string, 0, 3)
	if host := strings.TrimSpace(r.Host); host != "" {
		hosts = append(hosts, host)
	}
	for _, header := range []string{"X-Forwarded-Host", "X-Original-Host"} {
		for _, value := range strings.Split(r.Header.Get(header), ",") {
			host := strings.TrimSpace(value)
			if host != "" {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}

var errClosedSubscription = errors.New("subscription closed")

func decodeServerMessage(payload []byte) (serverMessage, error) {
	var message serverMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return serverMessage{}, err
	}
	if message.Op == "" {
		return serverMessage{}, errClosedSubscription
	}
	return message, nil
}
