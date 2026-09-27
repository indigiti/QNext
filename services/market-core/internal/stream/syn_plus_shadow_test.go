package stream

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

func TestSynPlusShadowBridgeAcceptsAuthenticatedSnapshot(t *testing.T) {
	t.Setenv("UPSTOX_ACCESS_TOKEN", "test-upstox-token")
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	feed := feedstatus.New()
	broker := NewBroker(32, 8)
	handler := NewWebSocketHandler(broker)

	snapshot := research.SynPlusSnapshot{
		Schema:             research.SynPlusSnapshotSchema,
		InstrumentID:       "QNEXT:NIFTY-SYN+",
		Version:            "nifty-syn-plus-v1",
		SnapshotAtMS:       time.Now().UTC().UnixMilli(),
		SourceCurrentTSMS:  time.Now().UTC().UnixMilli(),
		Expiry:             "2026-10-01",
		ATM:                25000,
		Spot:               25012.5,
		Value:              25010.25,
		BasisToSpot:        -2.25,
		Quality:            "GOOD",
		ValidCandidates:    9,
		RejectedCandidates: 2,
		MicropriceLegs:     18,
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/stream", bytes.NewReader(body))
	request.Header.Set(researchBridgeTokenHeader, researchBridgeToken())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}

	if _, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "15s"); !ok {
		t.Fatal("expected live SYN+ 15s bar")
	}
	telemetry := feed.Snapshot()
	instrument, ok := telemetry.Instruments["QNEXT:NIFTY-SYN+"]
	if !ok || instrument.Price != snapshot.Value {
		t.Fatalf("unexpected SYN+ telemetry: %+v", telemetry.Instruments)
	}
	status, ok := telemetry.Synthetics["QNEXT:NIFTY-SYN+"].(map[string]any)
	if !ok || status["mode"] != "SHADOW" || status["authority"] != false {
		t.Fatalf("unexpected SYN+ status: %#v", telemetry.Synthetics["QNEXT:NIFTY-SYN+"])
	}
}

func TestSynPlusShadowBridgeRejectsUnauthenticatedSnapshot(t *testing.T) {
	t.Setenv("UPSTOX_ACCESS_TOKEN", "test-upstox-token")
	feedstatus.New()
	handler := NewWebSocketHandler(NewBroker(32, 8))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/stream", bytes.NewBufferString(`{"schema":"QNEXT.RESEARCH.SYNTHETIC_SNAPSHOT/1"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
