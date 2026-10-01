package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestRequestedFiveSecondHealthSymbolsSupportsSingleAndBatch(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/5s-health?symbol=nifty-syn&symbols=NIFTY,NIFTY-SYN%2B&instrument_id=NSE_INDEX%7CNifty%2050", nil)
	requested, err := requestedFiveSecondHealthSymbols(request)
	if err != nil {
		t.Fatalf("parse request: %v", err)
	}

	for _, value := range []string{"NIFTY-SYN", "NIFTY", "NIFTY-SYN+", "NSE_INDEX|NIFTY 50"} {
		if _, ok := requested[value]; !ok {
			t.Fatalf("expected %q in request set: %#v", value, requested)
		}
	}
}

func TestMatchesFiveSecondHealthRequestBySymbolOrInstrument(t *testing.T) {
	instrument := symbol.Instrument{ID: "NSE_INDEX|Nifty 50", Symbol: "NIFTY", AssetClass: "index"}

	if !matchesFiveSecondHealthRequest(instrument, map[string]struct{}{"NIFTY": {}}) {
		t.Fatal("symbol filter should match")
	}
	if !matchesFiveSecondHealthRequest(instrument, map[string]struct{}{"NSE_INDEX|NIFTY 50": {}}) {
		t.Fatal("instrument id filter should match case-insensitively")
	}
	if matchesFiveSecondHealthRequest(instrument, map[string]struct{}{"BANKNIFTY": {}}) {
		t.Fatal("unrelated filter should not match")
	}
	if !matchesFiveSecondHealthRequest(instrument, nil) {
		t.Fatal("empty filter should preserve legacy all-symbol behavior")
	}
}

func TestRequestedFiveSecondHealthSymbolsRejectsWideFanout(t *testing.T) {
	values := make([]string, 0, fiveSecondHealthMaxSymbols+1)
	for i := 0; i < fiveSecondHealthMaxSymbols+1; i++ {
		values = append(values, "SYMBOL"+string(rune('A'+i)))
	}
	request := httptest.NewRequest("GET", "/api/v1/5s-health?symbols="+strings.Join(values, ","), nil)
	if _, err := requestedFiveSecondHealthSymbols(request); err == nil {
		t.Fatalf("expected more than %d requested symbols to be rejected", fiveSecondHealthMaxSymbols)
	}
}
