package demand

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/observability"
)

const defaultIdleTTL = 45 * time.Second

type Target interface {
	Activate(context.Context) error
	Deactivate(context.Context) error
}

type Controller interface {
	Acquire(string)
	Release(string)
}

type TargetStatus struct {
	InstrumentID string `json:"instrument_id"`
	Refs         int    `json:"refs"`
	Active       bool   `json:"active"`
	IdleUntilMS  int64  `json:"idle_until_ms,omitempty"`
	LastChangeMS int64  `json:"last_change_ms,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type Snapshot struct {
	IdleTTLMS     int64          `json:"idle_ttl_ms"`
	Targets       int            `json:"targets"`
	ActiveTargets int            `json:"active_targets"`
	TotalRefs     int            `json:"total_refs"`
	Details       []TargetStatus `json:"details"`
}

type entry struct {
	transition   sync.Mutex
	target       Target
	refs         int
	active       bool
	idleUntil    time.Time
	lastChange   time.Time
	lastError    string
	timer        *time.Timer
	timerVersion uint64
}

type Registry struct {
	mu      sync.Mutex
	idleTTL time.Duration
	entries map[string]*entry
}

func New(idleTTL time.Duration) *Registry {
	if idleTTL <= 0 {
		idleTTL = defaultIdleTTL
	}
	return &Registry{
		idleTTL: idleTTL,
		entries: make(map[string]*entry),
	}
}

var defaultRegistry = New(idleTTLFromEnv())

func init() {
	observability.SetDemandSource(func() any { return defaultRegistry.Snapshot() })
}

func Default() *Registry {
	return defaultRegistry
}

func Register(instrumentID string, target Target) {
	defaultRegistry.Register(instrumentID, target)
}

func (r *Registry) Register(instrumentID string, target Target) {
	instrumentID = strings.TrimSpace(instrumentID)
	if instrumentID == "" || target == nil {
		return
	}

	r.mu.Lock()
	item := r.entries[instrumentID]
	if item == nil {
		item = &entry{}
		r.entries[instrumentID] = item
	}
	item.target = target
	shouldActivate := item.refs > 0 && !item.active
	if shouldActivate {
		item.active = true
		item.idleUntil = time.Time{}
		item.lastChange = time.Now().UTC()
	}
	r.mu.Unlock()

	if shouldActivate {
		r.activate(instrumentID, item, target)
	}
}

func (r *Registry) Acquire(instrumentID string) {
	instrumentID = strings.TrimSpace(instrumentID)
	if instrumentID == "" {
		return
	}

	r.mu.Lock()
	item := r.entries[instrumentID]
	if item == nil {
		item = &entry{}
		r.entries[instrumentID] = item
	}
	item.refs++
	if item.timer != nil {
		item.timer.Stop()
		item.timer = nil
	}
	item.timerVersion++
	item.idleUntil = time.Time{}
	item.lastChange = time.Now().UTC()
	target := item.target
	shouldActivate := target != nil && !item.active
	if shouldActivate {
		item.active = true
	}
	r.mu.Unlock()

	if shouldActivate {
		r.activate(instrumentID, item, target)
	}
}

func (r *Registry) Release(instrumentID string) {
	instrumentID = strings.TrimSpace(instrumentID)
	if instrumentID == "" {
		return
	}

	r.mu.Lock()
	item := r.entries[instrumentID]
	if item == nil {
		r.mu.Unlock()
		return
	}
	if item.refs > 0 {
		item.refs--
	}
	item.lastChange = time.Now().UTC()
	if item.refs > 0 {
		r.mu.Unlock()
		return
	}
	if item.target == nil {
		delete(r.entries, instrumentID)
		r.mu.Unlock()
		return
	}
	if !item.active {
		r.mu.Unlock()
		return
	}

	item.timerVersion++
	version := item.timerVersion
	item.idleUntil = time.Now().UTC().Add(r.idleTTL)
	item.timer = time.AfterFunc(r.idleTTL, func() {
		r.expire(instrumentID, version)
	})
	r.mu.Unlock()
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapshot := Snapshot{IdleTTLMS: r.idleTTL.Milliseconds()}
	for instrumentID, item := range r.entries {
		if item.target == nil {
			continue
		}
		status := TargetStatus{
			InstrumentID: instrumentID,
			Refs:         item.refs,
			Active:       item.active,
			LastError:    item.lastError,
		}
		if !item.idleUntil.IsZero() {
			status.IdleUntilMS = item.idleUntil.UnixMilli()
		}
		if !item.lastChange.IsZero() {
			status.LastChangeMS = item.lastChange.UnixMilli()
		}
		snapshot.Targets++
		snapshot.TotalRefs += item.refs
		if item.active {
			snapshot.ActiveTargets++
		}
		snapshot.Details = append(snapshot.Details, status)
	}
	sort.Slice(snapshot.Details, func(i, j int) bool {
		return snapshot.Details[i].InstrumentID < snapshot.Details[j].InstrumentID
	})
	return snapshot
}

func (r *Registry) expire(instrumentID string, version uint64) {
	r.mu.Lock()
	item := r.entries[instrumentID]
	if item == nil || item.timerVersion != version || item.refs != 0 || !item.active || item.target == nil {
		r.mu.Unlock()
		return
	}
	item.active = false
	item.timer = nil
	item.idleUntil = time.Time{}
	item.lastChange = time.Now().UTC()
	target := item.target
	r.mu.Unlock()

	r.deactivate(instrumentID, item, target)
}

func (r *Registry) activate(instrumentID string, item *entry, target Target) {
	go func() {
		item.transition.Lock()
		defer item.transition.Unlock()

		if !r.transitionStillWanted(instrumentID, item, target, true) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := target.Activate(ctx); err != nil {
			r.setError(instrumentID, err.Error())
			return
		}
		r.setError(instrumentID, "")
	}()
}

func (r *Registry) deactivate(instrumentID string, item *entry, target Target) {
	go func() {
		item.transition.Lock()
		defer item.transition.Unlock()

		if !r.transitionStillWanted(instrumentID, item, target, false) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := target.Deactivate(ctx); err != nil {
			r.setError(instrumentID, err.Error())
			return
		}
		r.setError(instrumentID, "")
	}()
}

func (r *Registry) transitionStillWanted(instrumentID string, item *entry, target Target, active bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.entries[instrumentID]
	return current == item && item.target == target && item.active == active
}

func (r *Registry) setError(instrumentID, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item := r.entries[instrumentID]; item != nil {
		item.lastError = message
	}
}

func idleTTLFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("QNEXT_DEMAND_IDLE_TTL_MS"))
	if raw == "" {
		return defaultIdleTTL
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultIdleTTL
	}
	return time.Duration(value) * time.Millisecond
}
