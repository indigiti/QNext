package upstox

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

type SubscriptionState struct {
	mu      sync.RWMutex
	emitMu  sync.Mutex // serializes desired-key changes and their outbound updates
	guid    string
	mode    SubscriptionMode
	keys    map[string]bool
	keyMode map[string]SubscriptionMode
	updates chan SubscriptionRequest
}

func NewSubscriptionState(guid string, mode SubscriptionMode, initialKeys []string) (*SubscriptionState, error) {
	if strings.TrimSpace(guid) == "" {
		return nil, errors.New("subscription guid is required")
	}
	if !validSubscriptionMode(mode) {
		return nil, errors.New("unsupported subscription mode")
	}

	state := &SubscriptionState{
		guid:    guid,
		mode:    mode,
		keys:    make(map[string]bool),
		keyMode: make(map[string]SubscriptionMode),
		updates: make(chan SubscriptionRequest, 32),
	}
	for _, raw := range initialKeys {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, errors.New("initial subscription key is empty")
		}
		state.keys[key] = true
		state.keyMode[key] = state.modeForKey(key)
	}
	return state, nil
}

func (s *SubscriptionState) Snapshot() SubscriptionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Upstox applies one mode to the complete subscription request. During the
	// normal live session only dynamically-added derivative keys use Full mode.
	// On reconnect, if any such key is still active, subscribe the small current
	// set in Full mode so quote-driven synthetics do not silently fall back to
	// LTP until the next basket rotation. Once derivative demand is released,
	// the snapshot automatically returns to the base LTPC mode.
	mode := s.mode
	for key := range s.keys {
		if s.keyMode[key] == ModeFull {
			mode = ModeFull
			break
		}
	}
	return SubscriptionRequest{
		GUID:   s.guid,
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           mode,
			InstrumentKeys: sortedSubscriptionKeys(s.keys),
		},
	}
}

func (s *SubscriptionState) Updates() <-chan SubscriptionRequest {
	return s.updates
}

func (s *SubscriptionState) Subscribe(ctx context.Context, keys []string) error {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()

	// Compute requests before changing the desired subscription set. The
	// previous implementation committed keys first, then could fail to queue
	// the request, leaving the next retry convinced a silent key was enabled.
	byMode := make(map[SubscriptionMode][]string)
	added := make(map[string]SubscriptionMode)
	s.mu.RLock()
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || s.keys[key] {
			continue
		}
		if _, seen := added[key]; seen {
			continue
		}
		mode := s.modeForKey(key)
		added[key] = mode
		byMode[mode] = append(byMode[mode], key)
	}
	s.mu.RUnlock()

	var requests []SubscriptionRequest
	for _, mode := range []SubscriptionMode{ModeLTPC, ModeOptionGreeks, ModeFull, ModeFullD30} {
		selected := byMode[mode]
		if len(selected) == 0 {
			continue
		}
		sort.Strings(selected)
		requests = append(requests, SubscriptionRequest{
			GUID: s.guid, Method: MethodSubscribe,
			Data: SubscriptionData{Mode: mode, InstrumentKeys: selected},
		})
	}
	if len(requests) == 0 {
		return nil
	}
	if err := s.reserveOutbound(ctx, len(requests)); err != nil {
		return err
	}
	s.mu.Lock()
	for key, mode := range added {
		s.keys[key] = true
		s.keyMode[key] = mode
	}
	s.mu.Unlock()
	for _, request := range requests {
		s.updates <- request
	}
	return nil
}

// ReassertFull repairs an already-desired derivative stream that has gone
// silent while other Upstox instruments keep the WebSocket alive. Upstox V3
// supports change_mode for existing keys; do not subscribe them a second time,
// expand feed demand or modify the desired reconnect snapshot.
func (s *SubscriptionState) ReassertFull(ctx context.Context, keys []string) error {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.RLock()
	known := make(map[string]bool, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if s.keys[key] && s.keyMode[key] == ModeFull {
			known[key] = true
		}
	}
	s.mu.RUnlock()
	if len(known) == 0 {
		return errors.New("no active full-mode subscriptions to repair")
	}
	return s.emit(ctx, SubscriptionRequest{
		GUID:   s.guid,
		Method: MethodChangeMode,
		Data: SubscriptionData{
			Mode:           ModeFull,
			InstrumentKeys: sortedSubscriptionKeys(known),
		},
	})
}

// RenewFull is a guarded second-stage recovery for *existing* Full-mode
// futures subscriptions. It emits unsubscribe then subscribe as a reserved
// pair without changing the desired set (a reconnect still restores the key).
// No pair is queued unless both messages fit: a lone unsubscribe could leave
// an otherwise healthy contract offline when the outgoing writer is congested.
func (s *SubscriptionState) RenewFull(ctx context.Context, keys []string) error {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	selected := make(map[string]bool, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if !derivativeProviderKey(key) || !s.keys[key] || s.keyMode[key] != ModeFull {
			s.mu.RUnlock()
			return errors.New("cannot renew an absent or non-full futures subscription")
		}
		selected[key] = true
	}
	s.mu.RUnlock()
	if len(selected) == 0 {
		return errors.New("no enabled futures subscriptions to renew")
	}
	if err := s.reserveOutbound(ctx, 2); err != nil {
		return err
	}
	ordered := sortedSubscriptionKeys(selected)
	s.updates <- SubscriptionRequest{
		GUID: s.guid, Method: MethodUnsubscribe,
		Data: SubscriptionData{InstrumentKeys: ordered},
	}
	s.updates <- SubscriptionRequest{
		GUID: s.guid, Method: MethodSubscribe,
		Data: SubscriptionData{Mode: ModeFull, InstrumentKeys: ordered},
	}
	return nil
}

func (s *SubscriptionState) Unsubscribe(ctx context.Context, keys []string) error {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()

	var removed []string
	seen := make(map[string]bool, len(keys))
	s.mu.RLock()
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || !s.keys[key] || seen[key] {
			continue
		}
		seen[key] = true
		removed = append(removed, key)
	}
	s.mu.RUnlock()
	if len(removed) == 0 {
		return nil
	}
	if err := s.reserveOutbound(ctx, 1); err != nil {
		return err
	}
	sort.Strings(removed)
	s.mu.Lock()
	for _, key := range removed {
		delete(s.keys, key)
		delete(s.keyMode, key)
	}
	s.mu.Unlock()
	s.updates <- SubscriptionRequest{
		GUID: s.guid, Method: MethodUnsubscribe,
		Data: SubscriptionData{InstrumentKeys: removed},
	}
	return nil
}

func (s *SubscriptionState) modeForKey(key string) SubscriptionMode {
	if derivativeProviderKey(key) {
		return ModeFull
	}
	return s.mode
}

func derivativeProviderKey(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	segment, _, _ := strings.Cut(key, "|")
	return strings.HasSuffix(segment, "_FO")
}

func validSubscriptionMode(mode SubscriptionMode) bool {
	switch mode {
	case ModeLTPC, ModeOptionGreeks, ModeFull, ModeFullD30:
		return true
	default:
		return false
	}
}

// reserveOutbound must be called under emitMu. Because all writers hold
// emitMu and channel consumers can only remove messages, a successful
// reservation guarantees that the following batch cannot partially enqueue.
// It also avoids holding up subscription changes behind a disconnected writer.
func (s *SubscriptionState) reserveOutbound(ctx context.Context, count int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cap(s.updates)-len(s.updates) < count {
		return errors.New("subscription update queue has insufficient capacity")
	}
	return nil
}

func (s *SubscriptionState) emit(ctx context.Context, request SubscriptionRequest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.updates <- request:
		return nil
	}
}

func sortedSubscriptionKeys(keys map[string]bool) []string {
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
