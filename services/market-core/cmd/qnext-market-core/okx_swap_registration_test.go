package main

import (
	"testing"

	"github.com/indigiti/QNext/services/market-core/internal/provider/okx"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

func TestOKXSwapAndXPerpRegistrationsNeverCollide(t *testing.T) {
	r := symbol.NewRegistry()
	x := okx.Contract{InstrumentID: okx.InstrumentID, InstID: "XAU-USD_UM_XPERP-310502", ContractValue: 0.001}
	s := okx.Contract{InstrumentID: okx.SwapInstrumentID, InstID: okx.SwapProviderID, ContractValue: 0.001}
	if err := registerOKXGold(r, x); err != nil {
		t.Fatal(err)
	}
	if err := registerOKXSwap(r, s); err != nil {
		t.Fatal(err)
	}
	swap, ok := r.ResolveProviderKey(okx.ProviderName, okx.SwapProviderID)
	if !ok || swap.ID != okx.SwapInstrumentID || swap.Currency != "USDT" || swap.CalendarID != "OKX_CRYPTO_24_7" {
		t.Fatalf("incorrect swap metadata %+v", swap)
	}
	perp, ok := r.ResolveProviderKey(okx.ProviderName, x.InstID)
	if !ok || perp.ID != okx.InstrumentID || perp.Currency != "USD" {
		t.Fatalf("XPerp mapping polluted %+v", perp)
	}
	if len(r.ListVisible()) != 2 {
		t.Fatal("expected independent chartable gold instruments")
	}
}
