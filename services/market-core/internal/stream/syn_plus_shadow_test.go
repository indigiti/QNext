package stream

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/candle"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/feedstatus"
	"github.com/indigiti/QNext/services/market-core/internal/research"
)

func TestSynPlusShadowBridgeAcceptsAuthenticatedSnapshot(t *testing.T) {
	t.Setenv("UPSTOX_ACCESS_TOKEN", "test-upstox-token")
	t.Setenv("QNEXT_SYN_PLUS_INSTRUMENT_ID", "QNEXT:NIFTY-SYN+")
	feed := feedstatus.New()
	broker := NewBroker(32, 8)
	handler := NewWebSocketHandler(broker)

	snapshotAt := time.Date(2026, 9, 5, 4, 4, 7, 0, time.UTC)
	open15, _, err := candle.Bucket(snapshotAt, "15s")
	if err != nil {
		t.Fatal(err)
	}
	broker.PublishBar(domain.Bar{
		InstrumentID:      "NSE:NIFTY50",
		Timeframe:         "15s",
		OpenTime:          open15,
		CloseTime:         open15.Add(15 * time.Second),
		Open:              22580,
		High:              22585,
		Low:               22579,
		Close:             22584,
		Volume:            3770,
		Quality:           domain.QualityGood,
		AuthorityProvider: "upstox",
	})

	snapshot := research.SynPlusSnapshot{
		Schema:             research.SynPlusSnapshotSchema,
		InstrumentID:       "QNEXT:NIFTY-SYN+",
		Version:            "nifty-syn-plus-v1",
		SnapshotAtMS:       snapshotAt.UnixMilli(),
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

	bar, ok := broker.LatestBar("QNEXT:NIFTY-SYN+", "15s")
	if !ok {
		t.Fatal("expected live SYN+ 15s bar")
	}
	if bar.Volume != 3770 {
		t.Fatalf("expected mirrored NIFTY proxy volume, got %+v", bar)
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
