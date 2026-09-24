package upstox

import (
	"encoding/json"
	"errors"
	"strings"
)

type SubscriptionMethod string
type SubscriptionMode string

const (
	MethodSubscribe   SubscriptionMethod = "sub"
	MethodChangeMode  SubscriptionMethod = "change_mode"
	MethodUnsubscribe SubscriptionMethod = "unsub"

	ModeLTPC         SubscriptionMode = "ltpc"
	ModeOptionGreeks SubscriptionMode = "option_greeks"
	ModeFull         SubscriptionMode = "full"
	ModeFullD30      SubscriptionMode = "full_d30"
)

type SubscriptionRequest struct {
	GUID   string             `json:"guid"`
	Method SubscriptionMethod `json:"method"`
	Data   SubscriptionData   `json:"data"`
}

type SubscriptionData struct {
	Mode           SubscriptionMode `json:"mode,omitempty"`
	InstrumentKeys []string         `json:"instrumentKeys"`
}

func (r SubscriptionRequest) MarshalBinary() ([]byte, error) {
	if strings.TrimSpace(r.GUID) == "" {
		return nil, errors.New("subscription guid is required")
	}
	switch r.Method {
	case MethodSubscribe, MethodChangeMode, MethodUnsubscribe:
	default:
		return nil, errors.New("unsupported subscription method")
	}
	if len(r.Data.InstrumentKeys) == 0 {
		return nil, errors.New("at least one instrument key is required")
	}
	for _, key := range r.Data.InstrumentKeys {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("instrument keys must not be empty")
		}
	}

	if r.Method != MethodUnsubscribe {
		switch r.Data.Mode {
		case ModeLTPC, ModeOptionGreeks, ModeFull, ModeFullD30:
		default:
			return nil, errors.New("unsupported subscription mode")
		}
	}

	return json.Marshal(r)
}
