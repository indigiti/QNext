package upstox

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type optionContractsDoer func(*http.Request) (*http.Response, error)

func (f optionContractsDoer) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOptionContractsClient(t *testing.T) {
	client := OptionContractsClient{
		Endpoint: "https://example.test/option/contract",
		HTTP: optionContractsDoer(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get("instrument_key") != "NSE_INDEX|Nifty 50" {
				t.Fatalf("unexpected instrument key: %s", req.URL.RawQuery)
			}
			if req.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("missing auth header")
			}
			body := "{\"status\":\"success\",\"data\":[{\"name\":\"NIFTY\",\"segment\":\"NSE_FO\",\"exchange\":\"NSE\",\"expiry\":\"2026-09-29\",\"instrument_key\":\"NSE_FO|123\",\"trading_symbol\":\"NIFTY 25100 CE 29 SEP 26\",\"instrument_type\":\"CE\",\"underlying_key\":\"NSE_INDEX|Nifty 50\",\"underlying_symbol\":\"NIFTY\",\"strike_price\":25100,\"weekly\":true}]}"
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	contracts, err := client.Contracts(context.Background(), "token", "NSE_INDEX|Nifty 50")
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 || contracts[0].InstrumentKey != "NSE_FO|123" {
		t.Fatalf("unexpected contracts: %+v", contracts)
	}
}
