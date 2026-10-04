package upstox

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFutureContractsClient(t *testing.T) {
	client := FutureContractsClient{
		Endpoint: "https://example.test/instruments/search",
		HTTP: optionContractsDoer(func(req *http.Request) (*http.Response, error) {
			query := req.URL.Query()
			if query.Get("query") != "NIFTY" ||
				query.Get("exchanges") != "NSE" ||
				query.Get("segments") != "FUT" ||
				query.Get("instrument_types") != "FUT" ||
				query.Get("expiry") != "current_month,next_month" {
				t.Fatalf("unexpected future search query: %s", req.URL.RawQuery)
			}
			if req.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("missing auth header")
			}
			body := "{"status":"success","data":[{"name":"NIFTY","segment":"NSE_FO","exchange":"NSE","expiry":"2026-10-29","instrument_key":"NSE_FO|100","trading_symbol":"NIFTY FUT 29 OCT 26","instrument_type":"FUT","underlying_key":"NSE_INDEX|Nifty 50","underlying_symbol":"NIFTY","lot_size":65},{"name":"NIFTY","segment":"NSE_FO","exchange":"NSE","expiry":"2026-11-26","instrument_key":"NSE_FO|101","trading_symbol":"NIFTY FUT 26 NOV 26","instrument_type":"FUT","underlying_key":"NSE_INDEX|Nifty 50","underlying_symbol":"NIFTY","lot_size":65}]}"
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	contracts, err := client.Futures(context.Background(), "token", "NIFTY", "NSE")
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 ||
		contracts[0].InstrumentKey != "NSE_FO|100" ||
		contracts[1].InstrumentKey != "NSE_FO|101" {
		t.Fatalf("unexpected futures: %+v", contracts)
	}
}
