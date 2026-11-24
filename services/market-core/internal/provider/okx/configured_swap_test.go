package okx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverConfiguredLinearSwap(t *testing.T) {
	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"0.01", true},
		{"NaN", false},
		{"+Inf", false},
		{"0", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
					t.Errorf("wrong provider ID")
				}
				fmt.Fprintf(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","instType":"SWAP","state":"live","ctVal":"%s","ctValCcy":"BTC","ctType":"linear","settleCcy":"USDT"}]}`, tc.value)
			}))
			defer server.Close()
			contract, err := (Client{BaseURL: server.URL}).DiscoverLinearSwap(context.Background(), "BTC-USDT-SWAP")
			if (err == nil) != tc.ok {
				t.Fatalf("discovery: %v", err)
			}
			if tc.ok && (contract.InstrumentID != "OKX:BTC-USDT-SWAP" || contract.volumeSource() != "OKX_BTC_CONTRACT_TRADES") {
				t.Fatalf("wrong contract %+v", contract)
			}
		})
	}
}
