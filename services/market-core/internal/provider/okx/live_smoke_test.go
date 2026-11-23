package okx

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

// This gated test performs REAL public OKX API calls, not mocked fixtures.
// It is intentionally not part of deterministic unit-test jobs.
func TestLiveOKXGoldExternalSmoke(t *testing.T) {
	if os.Getenv("QNEXT_OKX_LIVE_SMOKE") != "1" {
		t.Skip("enable QNEXT_OKX_LIVE_SMOKE=1 for live exchange validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := Client{}
	contract, err := client.DiscoverXAUUSD(ctx)
	if err != nil {
		var response apiResponse[instrumentInfo]
		if diagErr := client.get(ctx, "/api/v5/public/instruments", url.Values{"instType": {"FUTURES"}}, &response); diagErr == nil {
			t.Logf("OKX exchange response code=%s, FUTURES count=%d", response.Code, len(response.Data))
			for _, item := range response.Data {
				if strings.Contains(strings.ToUpper(item.InstID), "XAU") || strings.Contains(strings.ToUpper(item.InstID), "GOLD") {
					t.Logf("gold candidate instId=%q type=%q state=%q ctVal=%q ctValCcy=%q", item.InstID, item.InstType, item.State, item.CtVal, item.CtValCcy)
				}
			}
		} else {
			t.Logf("diagnostic instruments query failed: %v", diagErr)
		}
		t.Fatalf("live OKX gold contract unavailable: %v", err)
	}
	t.Logf("verified OKX gold contract=%s contract_size_XAU=%v", contract.InstID, contract.ContractValue)
	now := time.Now().UTC()
	minute, err := client.History(ctx, contract, "1m", now.Add(-90*time.Minute), now.Add(-2*time.Minute))
	if err != nil || len(minute) == 0 {
		t.Fatalf("live 1m backfill unavailable: bars=%d error=%v", len(minute), err)
	}
	oneSecond, err := client.History(ctx, contract, "5s", now.Add(-3*time.Minute), now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("live 1s-based 5s repair unsupported: %v", err)
	}
	t.Logf("1m final bars=%d, contiguous 5s final recovery bars=%d", len(minute), len(oneSecond))
	subscribeAck := make(chan struct{}, 1)
	tradeReceived := make(chan domain.Tick, 1)
	runCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	stream := Stream{}
	done := make(chan error, 1)
	go func() {
		done <- stream.once(runCtx, contract, func(_ string, tick domain.Tick) error {
			select {
			case tradeReceived <- tick:
			default:
			}
			return nil
		}, func() {
			select {
			case subscribeAck <- struct{}{}:
			default:
			}
		})
	}()
	select {
	case <-subscribeAck:
		t.Log("trades-all WebSocket subscription acknowledged")
	case err := <-done:
		t.Fatalf("WebSocket ended before subscription: %v", err)
	case <-runCtx.Done():
		t.Fatalf("trades-all subscription timeout: %v", runCtx.Err())
	}
	select {
	case tick := <-tradeReceived:
		if tick.Provider != ProviderName || tick.Price <= 0 || tick.Quantity <= 0 {
			t.Fatalf("invalid real trade: %+v", tick)
		}
		t.Logf("actual XAUUSD trade time=%s price=%v quantity_XAU=%v", tick.EventTime, tick.Price, tick.Quantity)
	case err := <-done:
		t.Fatalf("WebSocket ended before individual gold trade: %v", err)
	case <-runCtx.Done():
		t.Fatalf("no individual trade within 30 seconds: %v", runCtx.Err())
	}
	stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WebSocket did not close")
	}
}
