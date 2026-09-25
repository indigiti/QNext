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
	guid    string
	mode    SubscriptionMode
	keys    map[string]bool
	updates chan SubscriptionRequest
}

func NewSubscriptionState(guid string, mode SubscriptionMode, initialKeys []string) (*SubscriptionState, error) {
	if strings.TrimSpace(guid) == "" {
		return nil, errors.New("subscription guid is required")
	}
	switch mode {
	case ModeLTPC, ModeOptionGreeks, ModeFull, ModeFullD30:
	default:
		return nil, errors.New("unsupported subscription mode")
	}

	state := &SubscriptionState{
		guid:    guid,
		mode:    mode,
		keys:    make(map[string]bool),
		updates: make(chan SubscriptionRequest, 32),
	}
	for _, raw := range initialKeys {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, errors.New("initial subscription key is empty")
		}
		state.keys[key] = true
	}
	return state, nil
}

func (s *SubscriptionState) Snapshot() SubscriptionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SubscriptionRequest{
		GUID:   s.guid,
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           s.mode,
			InstrumentKeys: sortedSubscriptionKeys(s.keys),
		},
	}
}

func (s *SubscriptionState) Updates() <-chan SubscriptionRequest {
	return s.updates
}

func (s *SubscriptionState) Subscribe(ctx context.Context, keys []string) error {
	s.mu.Lock()
	var added []string
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || s.keys[key] {
			continue
		}
		s.keys[key] = true
		added = append(added, key)
	}
	s.mu.Unlock()

	if len(added) == 0 {
		return nil
	}
	sort.Strings(added)
	return s.emit(ctx, SubscriptionRequest{
		GUID:   s.guid,
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           s.mode,
			InstrumentKeys: added,
		},
	})
}

func (s *SubscriptionState) Unsubscribe(ctx context.Context, keys []string) error {
	s.mu.Lock()
	var removed []string
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || !s.keys[key] {
			continue
		}
		delete(s.keys, key)
		removed = append(removed, key)
	}
	s.mu.Unlock()

	if len(removed) == 0 {
		return nil
	}
	sort.Strings(removed)
	return s.emit(ctx, SubscriptionRequest{
		GUID:   s.guid,
		Method: MethodUnsubscribe,
		Data: SubscriptionData{
			InstrumentKeys: removed,
		},
	})
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
