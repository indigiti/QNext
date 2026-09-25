package autolegs

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/synthetic"
)

type fakeResolver struct{}

func (fakeResolver) Resolve(_ context.Context, _ time.Time, strikes []float64) (Basket, error) {
	legs := make([]ResolvedLeg, 0, len(strikes)*2)
	for _, strike := range strikes {
		for _, side := range []synthetic.LegSide{synthetic.LegCall, synthetic.LegPut} {
			suffix := "CE"
			if side == synthetic.LegPut {
				suffix = "PE"
			}
			id := fmt.Sprintf("NIFTY:%.0f:%s", strike, suffix)
			legs = append(legs, ResolvedLeg{
				InstrumentID: id,
				ProviderKey:  "UPSTOX|" + id,
				Strike:       strike,
				Side:         side,
			})
		}
	}
	return Basket{Expiry: "2026-09-29", Legs: legs}, nil
}

type fakeSubscriptions struct {
	mu           sync.Mutex
	subscribed   map[string]bool
	unsubscribed []string
}

func newFakeSubscriptions() *fakeSubscriptions {
	return &fakeSubscriptions{subscribed: make(map[string]bool)}
}

func (f *fakeSubscriptions) Subscribe(_ context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, key := range keys {
		f.subscribed[key] = true
	}
	return nil
}

func (f *fakeSubscriptions) Unsubscribe(_ context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, key := range keys {
		delete(f.subscribed, key)
		f.unsubscribed = append(f.unsubscribed, key)
	}
	return nil
}

func TestDesiredATMUsesHysteresis(t *testing.T) {
	if got := desiredATM(25100, 25126, 50, 5); got != 25100 {
		t.Fatalf("expected hysteresis to retain 25100, got %.0f", got)
	}
	if got := desiredATM(25100, 25131, 50, 5); got != 25150 {
		t.Fatalf("expected upward roll to 25150, got %.0f", got)
	}
	if got := desiredATM(25150, 25119, 50, 5); got != 25100 {
		t.Fatalf("expected downward roll to 25100, got %.0f", got)
	}
}

func TestCenteredStrikesBuildWarmRing(t *testing.T) {
	got := centeredStrikes(25100, 50, 7)
	want := []float64{24950, 25000, 25050, 25100, 25150, 25200, 25250}
	if len(got) != len(want) {
		t.Fatalf("unexpected ring: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected ring at %d: got %.0f want %.0f", i, got[i], want[i])
		}
	}
}

func TestManagerAtomicallyRollsToNextATM(t *testing.T) {
	subs := newFakeSubscriptions()
	var seq uint64
	manager, err := New(Config{
		UnderlyingInstrumentID: "NSE:NIFTY50",
		SyntheticInstrumentID:  "QNEXT:NIFTY-SYN",
		Version:                "nifty-syn-v2",
		StrikeInterval:         50,
		ActiveStrikes:          5,
		WarmStrikes:            7,
		HysteresisPoints:       5,
		Confirmation:           0,
		MinimumValidCandidates: 3,
		MaxLegAge:              2 * time.Second,
		MaxLegTimeSkew:         time.Second,
	}, fakeResolver{}, subs, func() uint64 {
		seq++
		return seq
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = manager.Run(ctx) }()

	at := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	_, _, _ = manager.Apply(domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Price:        25110,
		EventTime:    at,
		Quality:      domain.QualityGood,
	})
	waitForPending(t, manager, 25100)

	var emitted bool
	for _, strike := range []float64{25000, 25050, 25100, 25150, 25200} {
		for _, side := range []string{"CE", "PE"} {
			price := 100.0
			if side == "PE" {
				price = 99.0
			}
			_, emittedNow, applyErr := manager.Apply(domain.Tick{
				InstrumentID:  fmt.Sprintf("NIFTY:%.0f:%s", strike, side),
				Price:         price,
				EventTime:     at.Add(100 * time.Millisecond),
				ReceivedTime:  at.Add(110 * time.Millisecond),
				ProcessedTime: at.Add(120 * time.Millisecond),
				Quality:       domain.QualityGood,
			})
			if applyErr != nil {
				t.Fatal(applyErr)
			}
			emitted = emitted || emittedNow
		}
	}
	if !emitted {
		t.Fatal("expected initial synthetic generation to become active")
	}
	if status := manager.Status(); status.ATM != 25100 || status.Expiry != "2026-09-29" {
		t.Fatalf("unexpected initial status: %+v", status)
	}

	_, _, _ = manager.Apply(domain.Tick{
		InstrumentID: "NSE:NIFTY50",
		Price:        25131,
		EventTime:    at.Add(time.Second),
		Quality:      domain.QualityGood,
	})
	waitForPending(t, manager, 25150)

	_, rolled, applyErr := manager.Apply(domain.Tick{
		InstrumentID:  "NIFTY:25100:CE",
		Price:         101,
		EventTime:     at.Add(1100 * time.Millisecond),
		ReceivedTime:  at.Add(1110 * time.Millisecond),
		ProcessedTime: at.Add(1120 * time.Millisecond),
		Quality:       domain.QualityGood,
	})
	if applyErr != nil {
		t.Fatal(applyErr)
	}
	if !rolled {
		t.Fatal("expected cached warm-ring data to permit atomic ATM roll")
	}
	if status := manager.Status(); status.ATM != 25150 || status.Generation < 2 {
		t.Fatalf("unexpected rolled status: %+v", status)
	}
}

func waitForPending(t *testing.T, manager *Manager, atm float64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if status := manager.Status(); status.PendingATM == atm {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pending ATM %.0f was not prepared; status=%+v", atm, manager.Status())
}
