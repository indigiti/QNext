package stream

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/marketconfig"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

const researchBridgeTokenHeader = "X-QNext-Research-Token"

type WebSocketHandler struct {
	Broker        *Broker
	AllowedOrigin func(*http.Request) bool
	shadow        *synPlusShadowBridge // NIFTY backwards-compatible bridge
	shadows       map[string]*synPlusShadowBridge
}

type synPlusShadowBridge struct {
	broker         *Broker
	engine         *candle.Engine
	instrumentID   string
	volumeSourceID string
	timeframes     []string
	sequence       atomic.Uint64

	mu         sync.RWMutex
	latest     research.SynPlusSnapshot
	receivedAt time.Time

	volumeMu     sync.Mutex
	volumeMirror map[string][]synPlusVolumeMirror
	recentBars   map[string][]domain.Bar
}

type synPlusVolumeMirror struct {
	openTime time.Time
	volume   float64
	revision uint32
	final    bool
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
	Op              string    `json:"op"`
	Protocol        string    `json:"protocol,omitempty"`
	Channel         string    `json:"channel,omitempty"`
	StreamID        string    `json:"stream_id,omitempty"`
	Seq             uint64    `json:"seq,omitempty"`
	Symbol          string    `json:"symbol,omitempty"`
	Timeframe       string    `json:"timeframe,omitempty"`
	Quality         string    `json:"quality,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	Code            string    `json:"code,omitempty"`
	Message         string    `json:"message,omitempty"`
	Bar             *wireBar  `json:"bar,omitempty"`
	LivePublishedAt time.Time `json:"-"`
}

type wireBar struct {
	Time     int64   `json:"time"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
	VolumeSource string `json:"volume_source"`
	VolumeStatus string `json:"volume_status"`
	Final    bool    `json:"final"`
	Revision uint32  `json:"revision"`
}

func NewWebSocketHandler(broker *Broker) http.Handler {
	handler := &WebSocketHandler{
		Broker:  broker,
		shadows: make(map[string]*synPlusShadowBridge),
	}
	for _, bridge := range newSynPlusShadowBridges(broker) {
		handler.shadows[strings.ToUpper(bridge.instrumentID)] = bridge
		if strings.EqualFold(bridge.instrumentID, niftySynPlusID()) {
			handler.shadow = bridge
		}
		// Each enabled index has a distinct canonical volume observer.
		// Registration must not evict another index's observer.
		broker.SetBarObserver(bridge.volumeSourceID, bridge.observeVolumeSource)
		feedstatus.SetDefaultSyntheticStatusFor(bridge.instrumentID, bridge.status)
	}
	return handler
}

func niftySynPlusID() string {
	id := strings.TrimSpace(os.Getenv("QNEXT_SYN_PLUS_INSTRUMENT_ID"))
	if id == "" {
		id = "QNEXT:NIFTY-SYN+"
	}
	return id
}

// SYN+ is enabled only for NIFTY by default. Additional configured indices
// must be explicitly opted in and supplied by independent collector processes.
// This avoids publishing empty shadow symbols for markets without rich feeds.
func enabledSynPlusMarkets() map[string]bool {
	configured := strings.TrimSpace(os.Getenv("QNEXT_SYN_PLUS_ENABLED_MARKETS"))
	if configured == "" {
		configured = "NIFTY"
	}
	enabled := make(map[string]bool)
	for _, symbol := range strings.Split(configured, ",") {
		if key := strings.ToUpper(strings.TrimSpace(symbol)); key != "" {
			enabled[key] = true
		}
	}
	return enabled
}

func newSynPlusShadowBridges(broker *Broker) []*synPlusShadowBridge {
	if broker == nil {
		return nil
	}
	timeframes := marketconfig.DefaultChartTimeframes()
	markets := marketconfig.DefaultMarkets()
	if path := strings.TrimSpace(os.Getenv("QNEXT_MARKET_CONFIG")); path != "" {
		if config, err := marketconfig.Load(path); err == nil {
			timeframes = config.EffectiveChartTimeframes()
			markets = config.EffectiveMarkets()
		}
	}
	enabled := enabledSynPlusMarkets()
	bridges := make([]*synPlusShadowBridge, 0, len(markets))
	for _, market := range markets {
		name := strings.ToUpper(strings.TrimSpace(market.Symbol))
		if !enabled[name] {
			continue
		}
		id := "QNEXT:" + name + "-SYN+"
		if name == "NIFTY" {
			id = niftySynPlusID()
		}
		bridges = append(bridges, &synPlusShadowBridge{
			broker:         broker,
			engine:         candle.New("candle-v2-session-aligned-shadow"),
			instrumentID:   id,
			volumeSourceID: market.Underlying.InstrumentID,
			timeframes:     append([]string(nil), timeframes...),
			volumeMirror:   make(map[string][]synPlusVolumeMirror),
			recentBars:     make(map[string][]domain.Bar),
		})
	}
	return bridges
}

// Retained for existing single-market tests and internal callers.
func newSynPlusShadowBridge(broker *Broker) *synPlusShadowBridge {
	for _, bridge := range newSynPlusShadowBridges(broker) {
		if strings.EqualFold(bridge.instrumentID, niftySynPlusID()) {
			return bridge
		}
	}
	return nil
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Broker == nil {
		http.Error(w, "stream broker unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodPost {
		h.serveSynPlusSnapshot(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	h.Broker.webSocketConnections.Add(1)
	defer h.Broker.webSocketConnections.Add(-1)
	defer connection.Close()

	var writeMu sync.Mutex
	write := func(message serverMessage) error {
		started := time.Now()
		writeMu.Lock()
		defer writeMu.Unlock()
		err := connection.WriteJSON(message)
		h.Broker.writeLatency.observe(time.Since(started))
		if err == nil && !message.LivePublishedAt.IsZero() {
			h.Broker.publishToWriteLatency.observe(time.Since(message.LivePublishedAt))
		}
		return err
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

func (h *WebSocketHandler) serveSynPlusSnapshot(w http.ResponseWriter, r *http.Request) {
	if len(h.shadows) == 0 {
		http.Error(w, "SYN+ shadow bridge unavailable", http.StatusServiceUnavailable)
		return
	}
	expected := researchBridgeToken()
	provided := strings.TrimSpace(r.Header.Get(researchBridgeTokenHeader))
	if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var snapshot research.SynPlusSnapshot
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&snapshot); err != nil {
		http.Error(w, "invalid SYN+ snapshot", http.StatusBadRequest)
		return
	}
	if snapshot.Schema != research.SynPlusSnapshotSchema ||
		strings.TrimSpace(snapshot.InstrumentID) == "" ||
		snapshot.SnapshotAtMS <= 0 {
		http.Error(w, "invalid SYN+ snapshot", http.StatusUnprocessableEntity)
		return
	}

	bridge := h.shadows[strings.ToUpper(strings.TrimSpace(snapshot.InstrumentID))]
	if bridge == nil {
		http.Error(w, "SYN+ market is not enabled", http.StatusUnprocessableEntity)
		return
	}
	if err := bridge.observe(snapshot); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "mode": "SHADOW"})
}

func (b *synPlusShadowBridge) observe(snapshot research.SynPlusSnapshot) error {
	b.mu.Lock()
	b.latest = snapshot
	b.receivedAt = time.Now().UTC()
	b.mu.Unlock()

	if snapshot.Value <= 0 {
		return nil
	}
	quality := domain.QualityDegraded
	if strings.EqualFold(snapshot.Quality, string(domain.QualityGood)) {
		quality = domain.QualityGood
	}
	tick := domain.Tick{
		InstrumentID:     b.instrumentID,
		Provider:         "qnext-syn-plus-shadow",
		Price:            snapshot.Value,
		EventTime:        time.UnixMilli(snapshot.SnapshotAtMS).UTC(),
		ReceivedTime:     time.Now().UTC(),
		Sequence:         b.sequence.Add(1),
		Quality:          quality,
		SyntheticVersion: snapshot.Version,
	}
	feedstatus.ObserveDefault(tick)
	if quality != domain.QualityGood {
		return nil
	}

	// Serialize shadow price updates with source-volume notifications so a
	// later price snapshot cannot overwrite already-mirrored candle volume.
	b.volumeMu.Lock()
	defer b.volumeMu.Unlock()
	for _, timeframe := range b.timeframes {
		// On restart, the source may already have a forming bar before the
		// first SYN+ snapshot. Seed the bounded cache from its live state.
		if b.volumeSourceID != "" {
			if source, ok := b.broker.LatestBar(b.volumeSourceID, timeframe); ok {
				b.rememberVolume(source)
			}
		}
		bars, err := b.engine.Apply(tick, timeframe)
		if err != nil {
			return err
		}
		for _, bar := range bars {
			if volume, ok := b.sourceVolume(bar.Timeframe, bar.OpenTime); ok {
				bar.Volume = volume
				if volume > 0 {
					bar.VolumeSource, bar.VolumeStatus = "FUTURES_PROXY", "OBSERVED"
				} else {
					bar.VolumeSource, bar.VolumeStatus = "", "UNKNOWN"
				}
			}
			if previous, ok := b.recentBar(bar.Timeframe, bar.OpenTime); ok && previous.Revision > bar.Revision {
				bar.Revision = previous.Revision
			}
			b.rememberBar(bar)
			b.broker.PublishBar(bar)
		}
	}
	return nil
}

// observeVolumeSource is invoked by the broker for each configured NIFTY
// source-volume candle. Unlike snapshot-triggered sampling, it also catches
// updates that arrive with no fresh SYN+ price observation and final 1m volume
// arriving shortly after the SYN+ price candle has rolled over.
func (b *synPlusShadowBridge) observeVolumeSource(source domain.Bar) {
	if source.InstrumentID != b.volumeSourceID {
		return
	}
	b.volumeMu.Lock()
	defer b.volumeMu.Unlock()
	if !b.rememberVolume(source) {
		return
	}
	bar, ok := b.recentBar(source.Timeframe, source.OpenTime)
	if !ok || bar.Volume == source.Volume {
		return
	}
	// Only the volume field is sourced from the NIFTY futures proxy.
	// Price OHLC, finality and synthetic lineage remain SYN+ owned.
	bar.Volume = source.Volume
	if source.Volume > 0 {
		bar.VolumeSource, bar.VolumeStatus = "FUTURES_PROXY", "OBSERVED"
	} else {
		bar.VolumeSource, bar.VolumeStatus = "", "UNKNOWN"
	}
	bar.Revision++
	if bar.Final {
		bar.Corrected = true
		bar.CorrectedAt = time.Now().UTC()
	}
	b.rememberBar(bar)
	b.broker.PublishBar(bar)
}

// Cache only the latest two buckets per timeframe. This is enough to align
// the current minute and its just-finalized predecessor without retaining
// arbitrary historical data or introducing extra market subscriptions.
func (b *synPlusShadowBridge) rememberVolume(source domain.Bar) bool {
	entries := b.volumeMirror[source.Timeframe]
	for i, previous := range entries {
		if !previous.openTime.Equal(source.OpenTime) {
			continue
		}
		if source.Revision < previous.revision ||
			(source.Revision == previous.revision && source.Volume < previous.volume) {
			// Out-of-order source updates may not reduce a newer cumulative
			// volume observation unless an explicit correction revision exists.
			return false
		}
		changed := source.Volume != previous.volume || source.Revision != previous.revision || (source.Final && !previous.final)
		entries[i] = synPlusVolumeMirror{
			openTime: source.OpenTime, volume: source.Volume,
			revision: source.Revision, final: source.Final,
		}
		b.volumeMirror[source.Timeframe] = entries
		return changed
	}
	entry := synPlusVolumeMirror{
		openTime: source.OpenTime, volume: source.Volume,
		revision: source.Revision, final: source.Final,
	}
	if len(entries) >= 2 {
		if !source.OpenTime.After(entries[0].openTime) {
			return false
		}
		entries = entries[1:]
	}
	entries = append(entries, entry)
	if len(entries) == 2 && entries[0].openTime.After(entries[1].openTime) {
		entries[0], entries[1] = entries[1], entries[0]
	}
	b.volumeMirror[source.Timeframe] = entries
	return true
}

func (b *synPlusShadowBridge) sourceVolume(timeframe string, open time.Time) (float64, bool) {
	for _, entry := range b.volumeMirror[timeframe] {
		if entry.openTime.Equal(open) {
			return entry.volume, true
		}
	}
	return 0, false
}

func (b *synPlusShadowBridge) recentBar(timeframe string, open time.Time) (domain.Bar, bool) {
	for _, bar := range b.recentBars[timeframe] {
		if bar.OpenTime.Equal(open) {
			return bar, true
		}
	}
	return domain.Bar{}, false
}

func (b *synPlusShadowBridge) rememberBar(bar domain.Bar) {
	entries := b.recentBars[bar.Timeframe]
	for i := range entries {
		if entries[i].OpenTime.Equal(bar.OpenTime) {
			entries[i] = bar
			b.recentBars[bar.Timeframe] = entries
			return
		}
	}
	if len(entries) >= 2 {
		if !bar.OpenTime.After(entries[0].OpenTime) {
			return
		}
		entries = entries[1:]
	}
	entries = append(entries, bar)
	if len(entries) == 2 && entries[0].OpenTime.After(entries[1].OpenTime) {
		entries[0], entries[1] = entries[1], entries[0]
	}
	b.recentBars[bar.Timeframe] = entries
}

func (b *synPlusShadowBridge) status() any {
	b.mu.RLock()
	latest := b.latest
	receivedAt := b.receivedAt
	b.mu.RUnlock()

	return map[string]any{
		"mode":                      "SHADOW",
		"authority":                 false,
		"chart_visible":             true,
		"instrument_id":             b.instrumentID,
		"version":                   latest.Version,
		"snapshot_at_ms":            latest.SnapshotAtMS,
		"source_current_ts_ms":      latest.SourceCurrentTSMS,
		"received_at_ms":            receivedAt.UnixMilli(),
		"expiry":                    latest.Expiry,
		"atm":                       latest.ATM,
		"spot":                      latest.Spot,
		"value":                     latest.Value,
		"basis_to_spot":             latest.BasisToSpot,
		"quality":                   latest.Quality,
		"valid_candidates":          latest.ValidCandidates,
		"rejected_candidates":       latest.RejectedCandidates,
		"microprice_legs":           latest.MicropriceLegs,
		"mid_legs":                  latest.MidLegs,
		"ltp_legs":                  latest.LTPLegs,
		"median_absolute_deviation": latest.MedianAbsoluteDeviation,
		"chart_timeframes":          append([]string(nil), b.timeframes...),
		"volume_source":             b.volumeSourceID,
	}
}

func researchBridgeToken() string {
	accessToken := strings.TrimSpace(os.Getenv("UPSTOX_ACCESS_TOKEN"))
	if accessToken == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("qnext-syn-plus-shadow:" + accessToken))
	return hex.EncodeToString(sum[:])
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
			message := updateMessage(event)
			message.LivePublishedAt = event.PublishedAt
			if err := write(message); err != nil {
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
		VolumeSource: bar.VolumeSource,
		VolumeStatus: func() string {
			if bar.VolumeStatus != "" { return bar.VolumeStatus }
			return "UNKNOWN"
		}(),
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
