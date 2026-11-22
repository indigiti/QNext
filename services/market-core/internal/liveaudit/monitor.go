// Package liveaudit audits published candle events without influencing market calculations.
package liveaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/stream"
)

const (
	recentLimit = 40
	checkDelay  = 3 * time.Second
)

var india = time.FixedZone("IST", 5*60*60+30*60)

var auditedFrames = map[string]time.Duration{
	"5s": 5 * time.Second, "15s": 15 * time.Second,
	"30s": 30 * time.Second, "1m": time.Minute,
}

// Alert is evidence of a published inconsistency, not proof of a broker error.
type Alert struct {
	AtMS           int64   `json:"at_ms"`
	Kind           string  `json:"kind"`
	InstrumentID   string  `json:"instrument_id"`
	Timeframe      string  `json:"timeframe,omitempty"`
	OpenTimeMS     int64   `json:"open_time_ms,omitempty"`
	SourceID       string  `json:"source_id,omitempty"`
	ExpectedVolume float64 `json:"expected_volume,omitempty"`
	ObservedVolume float64 `json:"observed_volume,omitempty"`
	Detail         string  `json:"detail"`
}

type Snapshot struct {
	Enabled           bool              `json:"enabled"`
	State             string            `json:"state"`
	ObservedEvents    uint64            `json:"observed_events"`
	LastEventAtMS     int64             `json:"last_event_at_ms,omitempty"`
	Counts            map[string]uint64 `json:"counts"`
	Recent            []Alert           `json:"recent"`
	AuditDropped      uint64            `json:"audit_dropped"`
	RawCaptureEnabled bool              `json:"raw_capture_enabled"`
	RawCaptureDropped uint64            `json:"raw_capture_dropped"`
	RawCaptureErrors  uint64            `json:"raw_capture_errors"`
	PersistErrors     uint64            `json:"persist_errors"`
	LastPersistError  string            `json:"last_persist_error,omitempty"`
}

type observed struct {
	bar domain.Bar
	at  time.Time
}

type series struct {
	seq      uint64
	current  *observed
	previous *observed
}

// Diagnostics returns capture enabled, capture drops, capture write errors,
// and audit-queue drops. The probe runs only on the monitor goroutine.
type Diagnostics func() (bool, uint64, uint64, uint64)

type Monitor struct {
	mu               sync.RWMutex
	root             string
	aliases          map[string]string
	targets          map[string]bool
	events           chan stream.BarEvent
	series           map[string]*series
	raised           map[string]bool
	raisedDay        string
	counts           map[string]uint64
	recent           []Alert
	observed         uint64
	lastEventMS      int64
	auditDropped     uint64
	rawEnabled       bool
	rawDropped       uint64
	rawErrors        uint64
	persistErrors    uint64
	lastPersistError string
	probe            Diagnostics
}

// New accepts SYN/SYN+ -> underlying source associations already authorized
// by the market configuration. Only these instruments and four small
// timeframes are audited. No extra market feed subscription is requested.
func New(storageRoot string, aliases map[string]string) *Monitor {
	owned := make(map[string]string, len(aliases))
	targets := make(map[string]bool, len(aliases)*2)
	for syn, source := range aliases {
		if syn == "" || source == "" {
			continue
		}
		owned[syn] = source
		targets[syn] = true
		targets[source] = true
	}
	return &Monitor{
		root:    filepath.Join(storageRoot, "live-integrity"),
		aliases: owned,
		targets: targets,
		events:  make(chan stream.BarEvent, 4096),
		series:  make(map[string]*series),
		raised:  make(map[string]bool),
		counts:  make(map[string]uint64),
		recent:  make([]Alert, 0, recentLimit),
	}
}

func (m *Monitor) Events() chan<- stream.BarEvent { return m.events }

func (m *Monitor) Targets() []string {
	result := make([]string, 0, len(m.targets))
	for id := range m.targets {
		result = append(result, id)
	}
	return result
}

func (m *Monitor) SetDiagnostics(fn Diagnostics) {
	m.mu.Lock()
	m.probe = fn
	m.mu.Unlock()
}

// Run observes an already-published nonblocking broker tap. It cannot change
// canonical OHLC, futures-volume attribution, chart updates, or order flow.
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.events:
			m.observe(event)
		case at := <-ticker.C:
			m.inspect(at.UTC())
		}
	}
}

func (m *Monitor) observe(event stream.BarEvent) {
	bar := event.Bar
	if !m.targets[bar.InstrumentID] || bar.OpenTime.IsZero() || auditedFrames[bar.Timeframe] == 0 {
		return
	}
	at := event.PublishedAt.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observed++
	if at.UnixMilli() > m.lastEventMS {
		m.lastEventMS = at.UnixMilli()
	}
	key := bar.InstrumentID + "|" + bar.Timeframe
	entry := m.series[key]
	if entry == nil {
		entry = &series{}
		m.series[key] = entry
	}
	if event.Seq > 0 && event.Seq <= entry.seq {
		return
	}
	if event.Seq > 0 {
		entry.seq = event.Seq
	}

	switch {
	case entry.current == nil:
		entry.current = &observed{bar: bar, at: at}
	case bar.OpenTime.After(entry.current.bar.OpenTime):
		previous := entry.current.bar
		duration := auditedFrames[bar.Timeframe]
		if previous.OpenTime.In(india).Format("2006-01-02") == bar.OpenTime.In(india).Format("2006-01-02") &&
			bar.OpenTime.Sub(previous.OpenTime) > duration {
			m.emit(at, Alert{Kind: "PUBLISHED_CANDLE_GAP", InstrumentID: bar.InstrumentID,
				Timeframe: bar.Timeframe, OpenTimeMS: bar.OpenTime.UnixMilli(),
				Detail: fmt.Sprintf("gap after candle %d; does not imply trades were missed", previous.OpenTime.UnixMilli())})
		}
		entry.previous = entry.current
		entry.current = &observed{bar: bar, at: at}
	case bar.OpenTime.Equal(entry.current.bar.OpenTime):
		m.checkRevision(entry.current.bar, bar, at)
		if bar.Revision >= entry.current.bar.Revision {
			entry.current = &observed{bar: bar, at: at}
		}
	case entry.previous != nil && bar.OpenTime.Equal(entry.previous.bar.OpenTime):
		m.checkRevision(entry.previous.bar, bar, at)
		if bar.Revision >= entry.previous.bar.Revision {
			entry.previous = &observed{bar: bar, at: at}
		}
	}
}

func (m *Monitor) checkRevision(old, next domain.Bar, at time.Time) {
	base := Alert{InstrumentID: next.InstrumentID, Timeframe: next.Timeframe,
		OpenTimeMS: next.OpenTime.UnixMilli(), ExpectedVolume: old.Volume,
		ObservedVolume: next.Volume}
	switch {
	case next.Revision < old.Revision:
		base.Kind = "STALE_REVISION_PUBLISHED"
		base.Detail = "older candle revision was published after a newer revision"
	case next.Revision == old.Revision && next.Volume+0.00001 < old.Volume:
		base.Kind = "VOLUME_REGRESSION"
		base.Detail = "cumulative candle volume decreased without a new revision"
	case next.Revision == old.Revision && old.Final &&
		(old.Open != next.Open || old.High != next.High || old.Low != next.Low ||
			old.Close != next.Close || math.Abs(old.Volume-next.Volume) > 0.00001):
		base.Kind = "UNREVISIONED_FINAL"
		base.Detail = "final candle changed without a new revision"
	default:
		return
	}
	m.emit(at, base)
}

func (m *Monitor) matching(instrumentID, timeframe string, open time.Time) *observed {
	state := m.series[instrumentID+"|"+timeframe]
	if state == nil {
		return nil
	}
	if state.current != nil && state.current.bar.OpenTime.Equal(open) {
		return state.current
	}
	if state.previous != nil && state.previous.bar.OpenTime.Equal(open) {
		return state.previous
	}
	return nil
}

// inspect waits for the synchronous SYN+ volume bridge to publish its
// correction before reporting a lag. Comparison uses observed futures-proxy
// volume ONLY; no cash-index or options-basket volume is treated as trade truth.
func (m *Monitor) inspect(now time.Time) {
	m.mu.RLock()
	probe := m.probe
	m.mu.RUnlock()
	var captureOn bool
	var rawDrops, rawErrors, auditDrops uint64
	if probe != nil {
		captureOn, rawDrops, rawErrors, auditDrops = probe()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.rawEnabled = captureOn
	if rawDrops > m.rawDropped {
		m.emit(now, Alert{Kind: "RAW_CAPTURE_LOSS", Detail: fmt.Sprintf("diagnostic raw frames dropped: %d -> %d; incident replay may be incomplete", m.rawDropped, rawDrops)})
	}
	if rawErrors > m.rawErrors {
		m.emit(now, Alert{Kind: "RAW_CAPTURE_WRITE_ERROR", Detail: fmt.Sprintf("diagnostic capture write errors: %d -> %d", m.rawErrors, rawErrors)})
	}
	if auditDrops > m.auditDropped {
		m.emit(now, Alert{Kind: "AUDIT_QUEUE_OVERFLOW", Detail: fmt.Sprintf("monitor event queue overflows: %d -> %d; monitoring is incomplete", m.auditDropped, auditDrops)})
	}
	m.rawDropped, m.rawErrors, m.auditDropped = rawDrops, rawErrors, auditDrops

	for synthetic, sourceID := range m.aliases {
		for timeframe := range auditedFrames {
			state := m.series[synthetic+"|"+timeframe]
			if state == nil {
				continue
			}
			for _, syntheticBar := range []*observed{state.current, state.previous} {
				if syntheticBar == nil {
					continue
				}
				source := m.matching(sourceID, timeframe, syntheticBar.bar.OpenTime)
				if source == nil || source.bar.VolumeStatus != "OBSERVED" ||
					source.bar.VolumeSource != "FUTURES_PROXY" || source.bar.Volume <= 0 ||
					source.bar.Volume <= syntheticBar.bar.Volume+0.00001 ||
					now.Sub(source.at) < checkDelay {
					continue
				}
				m.emit(now, Alert{Kind: "SYN_FUTURES_VOLUME_LAG", InstrumentID: synthetic,
					SourceID: sourceID, Timeframe: timeframe,
					OpenTimeMS:     syntheticBar.bar.OpenTime.UnixMilli(),
					ExpectedVolume: source.bar.Volume, ObservedVolume: syntheticBar.bar.Volume,
					Detail: "published synthetic candle trails an observed futures-proxy source; check subsequent revisions"})
			}
		}
	}
}

func (m *Monitor) emit(at time.Time, alert Alert) {
	day := at.In(india).Format("2006-01-02")
	if m.raisedDay != day {
		m.raisedDay = day
		m.raised = make(map[string]bool)
	}
	id := fmt.Sprintf("%s|%s|%s|%d", alert.Kind, alert.InstrumentID, alert.Timeframe, alert.OpenTimeMS)
	// Non-bar diagnostics have no candle key; record each new counter
	// transition while suppressing unchanged totals between polls.
	if alert.InstrumentID == "" {
		id += "|" + alert.Detail
	}
	// A published candle produces at most one alert of each kind per session.
	// Counter transitions are distinct evidence events; unchanged counts are not.
	if m.raised[id] {
		return
	}
	m.raised[id] = true
	alert.AtMS = at.UnixMilli()
	m.counts[alert.Kind]++
	if len(m.recent) == recentLimit {
		copy(m.recent, m.recent[1:])
		m.recent[len(m.recent)-1] = alert
	} else {
		m.recent = append(m.recent, alert)
	}
	data, err := json.Marshal(alert)
	if err == nil {
		path := filepath.Join(m.root, at.In(india).Format("2006-01"), at.In(india).Format("2006-01-02")+".jsonl")
		if err = os.MkdirAll(filepath.Dir(path), 0750); err == nil {
			var file *os.File
			file, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
			if err == nil {
				_, err = file.Write(append(data, '\n'))
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
	}
	if err != nil {
		m.persistErrors++
		m.lastPersistError = err.Error()
	}
	log.Printf("QNext live integrity %s instrument=%s timeframe=%s open=%d: %s", alert.Kind,
		alert.InstrumentID, alert.Timeframe, alert.OpenTimeMS, alert.Detail)
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	counts := make(map[string]uint64, len(m.counts))
	for key, value := range m.counts {
		counts[key] = value
	}
	recent := append([]Alert(nil), m.recent...)
	state := "OBSERVING"
	if len(recent) > 0 || m.persistErrors > 0 {
		state = "ALERTS_RECORDED"
	}
	return Snapshot{
		Enabled: true, State: state, ObservedEvents: m.observed, LastEventAtMS: m.lastEventMS,
		Counts: counts, Recent: recent, AuditDropped: m.auditDropped,
		RawCaptureEnabled: m.rawEnabled, RawCaptureDropped: m.rawDropped,
		RawCaptureErrors: m.rawErrors, PersistErrors: m.persistErrors,
		LastPersistError: m.lastPersistError,
	}
}
