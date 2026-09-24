package upstox

import (
	"encoding/json"
	"testing"
)

func TestSubscriptionMarshalsProviderShape(t *testing.T) {
	payload, err := (SubscriptionRequest{
		GUID:   "qnext-test",
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           ModeLTPC,
			InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		},
	}).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["guid"] != "qnext-test" || decoded["method"] != "sub" {
		t.Fatalf("unexpected payload: %s", payload)
	}
	data := decoded["data"].(map[string]any)
	if data["mode"] != "ltpc" {
		t.Fatalf("unexpected mode: %v", data["mode"])
	}
}

func TestUnsubscribeDoesNotRequireMode(t *testing.T) {
	_, err := (SubscriptionRequest{
		GUID:   "qnext-test",
		Method: MethodUnsubscribe,
		Data: SubscriptionData{
			InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		},
	}).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
}
