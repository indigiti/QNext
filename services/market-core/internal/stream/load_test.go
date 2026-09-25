package stream

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBrokerFanoutTo100Subscribers(t *testing.T) {
	broker := NewBroker(512, 512)
	const subscriberCount = 100
	const eventCount = 256

	subscriptions := make([]*Subscription, 0, subscriberCount)
	for i := 0; i < subscriberCount; i++ {
		subscription, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
		if err != nil {
			t.Fatal(err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	defer func() {
		for _, subscription := range subscriptions {
			subscription.Cancel()
		}
	}()

	base := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	for i := 0; i < eventCount; i++ {
		broker.PublishBar(bar(
			"NSE:NIFTY50",
			"15s",
			base.Add(time.Duration(i)*time.Second),
			25100+float64(i),
		))
	}

	for subscriberIndex, subscription := range subscriptions {
		for want := uint64(1); want <= eventCount; want++ {
			select {
			case event, ok := <-subscription.Events:
				if !ok {
					t.Fatalf("subscriber %d was evicted at seq %d", subscriberIndex, want)
				}
				if event.Seq != want {
					t.Fatalf("subscriber %d seq=%d want=%d", subscriberIndex, event.Seq, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("subscriber %d timed out at seq %d", subscriberIndex, want)
			}
		}
	}

	snapshot := broker.Snapshot()
	if snapshot.Streams != 1 || snapshot.Subscribers != subscriberCount {
		t.Fatalf("unexpected broker snapshot: %+v", snapshot)
	}
}

func TestSlowSubscriberIsEvictedWithoutAffectingHealthySubscriber(t *testing.T) {
	broker := NewBroker(32, 2)
	slow, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Cancel()
	healthy, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer healthy.Cancel()

	const eventCount = 8
	received := make(chan []uint64, 1)
	go func() {
		sequences := make([]uint64, 0, eventCount)
		for len(sequences) < eventCount {
			event, ok := <-healthy.Events
			if !ok {
				received <- sequences
				return
			}
			sequences = append(sequences, event.Seq)
		}
		received <- sequences
	}()

	base := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	for i := 0; i < eventCount; i++ {
		broker.PublishBar(bar("NSE:NIFTY50", "15s", base.Add(time.Duration(i)*time.Second), 25100+float64(i)))
	}

	select {
	case sequences := <-received:
		if len(sequences) != eventCount || sequences[eventCount-1] != eventCount {
			t.Fatalf("healthy subscriber lost fan-out events: %+v", sequences)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("healthy subscriber stalled behind slow subscriber")
	}

	if got := broker.Snapshot().Subscribers; got != 1 {
		t.Fatalf("expected only healthy subscriber to remain, got %d", got)
	}

	for range slow.Events {
	}
	reason := ""
	select {
	case reason = <-slow.closeReason:
	default:
	}
	if reason != "slow_consumer" {
		t.Fatalf("slow subscriber close reason=%q", reason)
	}

	after := uint64(0)
	resumed, err := broker.Subscribe("NSE:NIFTY50", "15s", &after)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Cancel()
	if resumed.ResyncRequired || len(resumed.Replay) != eventCount {
		t.Fatalf("retained replay should recover slow subscriber: resync=%v replay=%d", resumed.ResyncRequired, len(resumed.Replay))
	}
}

func TestForwardSubscriptionSignalsSlowConsumerResync(t *testing.T) {
	broker := NewBroker(16, 1)
	subscription, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cancel()

	at := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	broker.PublishBar(bar("NSE:NIFTY50", "15s", at, 25100))
	broker.PublishBar(bar("NSE:NIFTY50", "15s", at.Add(time.Second), 25101))

	messages := make(chan serverMessage, 4)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		forwardSubscription(done, subscription, func(message serverMessage) error {
			messages <- message
			return nil
		})
		close(finished)
	}()

	var got []serverMessage
	for {
		select {
		case message := <-messages:
			got = append(got, message)
		case <-finished:
			if len(got) != 2 {
				t.Fatalf("expected update then resync, got %+v", got)
			}
			if got[0].Op != "update" || got[0].Seq != 1 {
				t.Fatalf("unexpected retained update: %+v", got[0])
			}
			if got[1].Op != "resync_required" || got[1].Reason != "slow_consumer" {
				t.Fatalf("unexpected slow-consumer control: %+v", got[1])
			}
			return
		case <-time.After(2 * time.Second):
			t.Fatal("forwarder did not terminate after slow-consumer eviction")
		}
	}
}

func TestWebSocketFanoutTo50Clients(t *testing.T) {
	broker := NewBroker(128, 128)
	server := httptest.NewServer(NewWebSocketHandler(broker))
	defer server.Close()

	const clientCount = 50
	const eventCount = 32
	connections := make([]*websocket.Conn, 0, clientCount)

	for i := 0; i < clientCount; i++ {
		connection, _, err := websocket.DefaultDialer.Dial(
			"ws"+strings.TrimPrefix(server.URL, "http"),
			nil,
		)
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		connections = append(connections, connection)

		var hello serverMessage
		if err := connection.ReadJSON(&hello); err != nil || hello.Op != "hello" {
			t.Fatalf("client %d hello=%+v err=%v", i, hello, err)
		}
		if err := connection.WriteJSON(clientMessage{
			Op: "subscribe", Channel: "bars", Symbol: "NSE:NIFTY50", Timeframe: "15s",
		}); err != nil {
			t.Fatalf("subscribe client %d: %v", i, err)
		}
		var subscribed serverMessage
		if err := connection.ReadJSON(&subscribed); err != nil || subscribed.Op != "subscribed" {
			t.Fatalf("client %d subscribed=%+v err=%v", i, subscribed, err)
		}
	}
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()

	var wg sync.WaitGroup
	errorsCh := make(chan string, clientCount)
	for i, connection := range connections {
		wg.Add(1)
		go func(clientIndex int, connection *websocket.Conn) {
			defer wg.Done()
			_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
			for want := uint64(1); want <= eventCount; want++ {
				var message serverMessage
				if err := connection.ReadJSON(&message); err != nil {
					errorsCh <- "client read failed"
					return
				}
				if message.Op != "update" || message.Seq != want {
					errorsCh <- "client received out-of-sequence update"
					return
				}
			}
		}(i, connection)
	}

	base := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	for i := 0; i < eventCount; i++ {
		broker.PublishBar(bar("NSE:NIFTY50", "15s", base.Add(time.Duration(i)*time.Second), 25100+float64(i)))
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(6 * time.Second):
		t.Fatal("50-client WebSocket fan-out timed out")
	}
	close(errorsCh)
	for message := range errorsCh {
		t.Fatal(message)
	}
}

func BenchmarkBrokerFanout100(b *testing.B) {
	broker := NewBroker(1, 1)
	const subscriberCount = 100
	subscriptions := make([]*Subscription, 0, subscriberCount)
	for i := 0; i < subscriberCount; i++ {
		subscription, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
		if err != nil {
			b.Fatal(err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	defer func() {
		for _, subscription := range subscriptions {
			subscription.Cancel()
		}
	}()

	at := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.PublishBar(bar("NSE:NIFTY50", "15s", at, float64(i)))
		for _, subscription := range subscriptions {
			<-subscription.Events
		}
	}
}
