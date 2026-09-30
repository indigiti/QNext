package stream

import (
	"context"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/demand"
)

type brokerDemandTarget struct{}

func (brokerDemandTarget) Activate(context.Context) error   { return nil }
func (brokerDemandTarget) Deactivate(context.Context) error { return nil }

func TestBrokerSubscriptionsReferenceCountLiveDemand(t *testing.T) {
	instrumentID := "QNEXT:BROKER-DEMAND-TEST"
	demand.Register(instrumentID, brokerDemandTarget{})
	broker := NewBroker(8, 8)

	first, err := broker.Subscribe(instrumentID, "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := broker.Subscribe(instrumentID, "30s", nil)
	if err != nil {
		first.Cancel()
		t.Fatal(err)
	}

	waitForDemandRefs(t, instrumentID, 2)
	first.Cancel()
	waitForDemandRefs(t, instrumentID, 1)
	second.Cancel()
	waitForDemandRefs(t, instrumentID, 0)
}

func waitForDemandRefs(t *testing.T, instrumentID string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, status := range demand.Default().Snapshot().Details {
			if status.InstrumentID == instrumentID && status.Refs == want {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("demand refs for %s did not reach %d; snapshot=%+v", instrumentID, want, demand.Default().Snapshot())
}
