package upstox

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

type fakeAuthorizer struct{ uri string }

func (f fakeAuthorizer) AuthorizedURI(context.Context, string) (string, error) {
	return f.uri, nil
}

type fakeDialer struct{ connection *fakeConnection }

func (f fakeDialer) Dial(_ context.Context, uri string) (BinaryConnection, error) {
	if uri != "wss://feed.example.test/one-time" {
		return nil, errors.New("unexpected uri")
	}
	return f.connection, nil
}

type fakeConnection struct {
	writes [][]byte
	frames [][]byte
}

func (f *fakeConnection) WriteBinary(_ context.Context, payload []byte) error {
	f.writes = append(f.writes, append([]byte(nil), payload...))
	return nil
}

func (f *fakeConnection) ReadBinary(context.Context) ([]byte, error) {
	if len(f.frames) == 0 {
		return nil, io.EOF
	}
	payload := f.frames[0]
	f.frames = f.frames[1:]
	return payload, nil
}

func (f *fakeConnection) Close() error { return nil }

type fakeDecoder struct {
	envelope DecodedEnvelope
}

func (f fakeDecoder) Decode([]byte) (DecodedEnvelope, error) {
	return f.envelope, nil
}

func TestWireClientOpensAndNormalizesFrame(t *testing.T) {
	registry := symbol.NewRegistry()
	if err := registry.Register(symbol.Instrument{
		ID: "NSE:NIFTY50", Symbol: "NIFTY", Name: "Nifty 50",
		AssetClass: "INDEX", Exchange: "NSE", Currency: "INR", Timezone: "Asia/Kolkata",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(symbol.ProviderInstrument{
		Provider: ProviderName, InstrumentID: "NSE:NIFTY50", ProviderKey: "NSE_INDEX|Nifty 50",
	}); err != nil {
		t.Fatal(err)
	}
	normalizer, err := NewNormalizer(registry)
	if err != nil {
		t.Fatal(err)
	}

	connection := &fakeConnection{frames: [][]byte{{1, 2, 3}}}
	sequence := uint64(0)
	client := &WireClient{
		Authorizer: fakeAuthorizer{uri: "wss://feed.example.test/one-time"},
		Dialer:     fakeDialer{connection: connection},
		Decoder: fakeDecoder{envelope: DecodedEnvelope{
			Type:      "live_feed",
			CurrentTS: "1740729566039",
			Feeds: map[string]Feed{
				"NSE_INDEX|Nifty 50": {
					LTPC: &LTPC{LTP: 219.3, LTT: "1740729552723", LTQ: "75", CP: 494.05},
				},
			},
		}},
		Normalizer: normalizer,
		NextSequence: func() uint64 {
			sequence++
			return sequence
		},
		Now: func() time.Time { return time.UnixMilli(1740729566045).UTC() },
	}

	request := SubscriptionRequest{
		GUID:   "qnext-test",
		Method: MethodSubscribe,
		Data: SubscriptionData{
			Mode:           ModeLTPC,
			InstrumentKeys: []string{"NSE_INDEX|Nifty 50"},
		},
	}

	var received []domain.Tick
	err = client.Run(context.Background(), "token", request, func(tick domain.Tick) error {
		received = append(received, tick)
		return nil
	})
	if !errors.Is(err, io.EOF) && err == nil {
		t.Fatalf("expected stream end error, got %v", err)
	}
	if len(connection.writes) != 1 {
		t.Fatalf("expected one binary subscription write, got %d", len(connection.writes))
	}
	if len(received) != 1 || received[0].InstrumentID != "NSE:NIFTY50" || received[0].Sequence != 1 {
		t.Fatalf("unexpected normalized ticks: %+v", received)
	}
}
