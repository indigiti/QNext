package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/history"
)

func historyLoadBar(at time.Time, close float64) domain.Bar {
	return domain.Bar{
		InstrumentID:      "NSE:NIFTY50",
		Timeframe:         "1m",
		OpenTime:          at,
		CloseTime:         at.Add(time.Minute),
		Open:              close,
		High:              close + 1,
		Low:               close - 1,
		Close:             close,
		Volume:            100,
		Final:             true,
		Quality:           domain.QualityGood,
		AuthorityProvider: "load-fixture",
	}
}

func TestBarsConcurrentHistoryLoadWhileFinalsPersist(t *testing.T) {
	store := history.New(t.TempDir())
	now := time.Now().UTC()
	base := time.Date(now.Year(), now.Month(), now.Day(), 1, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		if err := store.AppendBar(historyLoadBar(base.Add(time.Duration(i)*time.Minute), 25000+float64(i))); err != nil {
			t.Fatal(err)
		}
	}

	handler := New(store, Options{})
	writer := history.NewAsyncWriter(store, 128)
	writeDone := make(chan error, 1)
	go func() {
		for i := 20; i < 60; i++ {
			if err := writer.AppendBar(historyLoadBar(base.Add(time.Duration(i)*time.Minute), 25000+float64(i))); err != nil {
				writeDone <- err
				return
			}
			time.Sleep(100 * time.Microsecond)
		}
		writeDone <- nil
	}()

	url := fmt.Sprintf(
		"/api/v1/bars?instrument_id=NSE%%3ANIFTY50&timeframe=1m&from_ms=%d&to_ms=%d",
		base.UnixMilli(), base.Add(61*time.Minute).UnixMilli(),
	)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodGet, url, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Errorf("bars status=%d body=%s", response.Code, response.Body.String())
				return
			}
			var payload barsResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Errorf("decode bars: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, url, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("final bars status=%d body=%s", response.Code, response.Body.String())
	}
	var payload barsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Bars) != 60 {
		t.Fatalf("expected all 60 finalized bars after drain, got %d", len(payload.Bars))
	}
	if stats := store.CacheStats(); stats.DiskLoads != 1 {
		t.Fatalf("100 concurrent /bars requests caused repeated JSONL scans: %+v", stats)
	}
}
