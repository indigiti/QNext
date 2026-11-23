package okx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestDiscoverXAUUSDRequiresOneLiveXPerp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/public/instruments" || r.URL.Query().Get("instType") != "FUTURES" {
			t.Errorf("unexpected request: %s", r.URL)
			return
		}
		fmt.Fprint(w, `{"code":"0","data":[
			{"instId":"XAU-USD_UM_XPERP-310502","instType":"FUTURES","state":"live","ctVal":"0.001","ctValCcy":"XAU"},
			{"instId":"XAU-USD_UM-261127","instType":"FUTURES","state":"live","ctVal":"0.001","ctValCcy":"XAU"},
			{"instId":"XAU-USDT-SWAP","instType":"SWAP","state":"live","ctVal":"0.01","ctValCcy":"XAU"},
			{"instId":"XAU-USD_UM-251127_XPERP","instType":"FUTURES","state":"expired","ctVal":"0.001","ctValCcy":"XAU"}
		]}`)
	}))
	defer server.Close()
	c, err := (Client{BaseURL: server.URL}).DiscoverXAUUSD(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.InstID != "XAU-USD_UM_XPERP-310502" || c.ContractValue != 0.001 {
		t.Fatalf("wrong contract: %+v", c)
	}
}

func TestDiscoverXAUUSDRejectsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":"0","data":[{"instId":"XAU-USDT-SWAP","instType":"SWAP","state":"live","ctVal":"0.001","ctValCcy":"XAU"}]}`)
	}))
	defer server.Close()
	_, err := (Client{BaseURL: server.URL}).DiscoverXAUUSD(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing to guess") {
		t.Fatalf("should fail closed: %v", err)
	}
}

func TestHistoryConfirmedOnlyAndContractToGoldVolume(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("bar") != "1m" {
			t.Errorf("bad bar %s", r.URL.Query().Get("bar"))
		}
		fmt.Fprintf(w, `{"code":"0","data":[["%d","4000","4010","3990","4005","10","0","0","1"],["%d","4005","4011","4000","4009","20","0","0","0"]]}`,
			now.Add(-time.Minute).UnixMilli(), now.UnixMilli())
	}))
	defer server.Close()
	c := Client{BaseURL: server.URL}
	bars, err := c.History(context.Background(), Contract{InstID: "XAU-USD_UM_XPERP-310502", ContractValue: 0.001},
		"1m", now.Add(-3*time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Volume != 0.01 || bars[0].AuthorityProvider != ProviderName || !bars[0].Final {
		t.Fatalf("unexpected history %+v", bars)
	}
}

func TestFiveSecondRecoveryNeverFillsMissingSeconds(t *testing.T) {
	start := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	source := make([]domain.Bar, 0, 10)
	for i := 0; i < 10; i++ {
		if i == 7 {
			continue
		}
		open := start.Add(time.Duration(i) * time.Second)
		source = append(source, domain.Bar{InstrumentID: InstrumentID, Timeframe: "1s", OpenTime: open, CloseTime: open.Add(time.Second), Open: 4000, High: 4010, Low: 3990, Close: 4005, Volume: 0.001, Final: true})
	}
	bars := fiveSecondFromOneSecond(source)
	if len(bars) != 1 || !bars[0].OpenTime.Equal(start) || bars[0].Volume < 0.004999 || bars[0].Volume > 0.005001 {
		t.Fatalf("missing seconds must never generate a false 5s bar: %+v", bars)
	}
}
