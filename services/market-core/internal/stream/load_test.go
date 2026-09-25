package stream

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBrokerFanoutCertification(t *testing.T) {
	for _, subscriberCount := range []int{1, 10, 50, 100} {
		t.Run(fmt.Sprintf("%d_subscribers", subscriberCount), func(t *testing.T) {
			broker := NewBroker(1024, 128)
			subs := make([]*Subscription, 0, subscriberCount)
			for i := 0; i < subscriberCount; i++ {
				sub, err := broker.Subscribe("NSE:NIFTY50", "15s", nil)
				if err != nil {
					t.Fatal(err)
				}
				subs = append(subs, sub)
			}
			defer func() {
				for _, sub := range subs {
					sub.Cancel()
				}
			}()

			const eventCount = 64
			var wg sync.WaitGroup
			errCh := make(chan error, subscriberCount)
			for _, sub := range subs {
				sub := sub
				wg.Add(1)
				go func() {
					defer wg.Done()
					for want := uint64(1); want <= eventCount; want++ {
						select {
						case event, ok := <-sub.Events:
							if !ok {
								errCh <- fmt.Errorf("subscription closed at seq %d", want)
								return
							}
							if event.Seq != want {
								errCh <- fmt.Errorf("got seq %d want %d", event.Seq, want)
								return
							}
						case <-time.After(2 * time.Second):
							errCh <- fmt.Errorf("timed out waiting for seq %d", want)
							return
						}
					}
				}()
			}

			base := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
			for i := 0; i < eventCount; i++ {
				broker.PublishBar(bar(
					"NSE:NIFTY50",
					"15s",
					base.Add(time.Duration(i)*time.Second),
					25100+float64(i),
				))
			}

			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("fanout certification timed out")
			}
			close(errCh)
			for err := range errCh {
				if err != nil {
					t.Fatal(err)
				}
			}

			snapshot := broker.Snapshot()
			if snapshot.ActiveSubscribers != subscriberCount {
				t.Fatalf("active subscribers=%d want %d", snapshot.ActiveSubscribers, subscriberCount)
			}
			if snapshot.PublishedEvents != eventCount {
				t.Fatalf("published events=%d want %d", snapshot.PublishedEvents, eventCount)
			}
			if snapshot.SlowSubscriberDrops != 0 {
				t.Fatalf("healthy fanout must not drop subscribers: %+v", snapshot)
			}
		})
	}
}

func TestBrokerEvictsSlowSubscriberAndSupportsResume(t *testing.T) {
	broker := NewBroker(8, 2)
	slow, err := broker.Subscribe("NSE:NIFTY50", "1m", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Cancel()

	base := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		broker.PublishBar(bar(
			"NSE:NIFTY50",
			"1m",
			base.Add(time.Duration(i)*time.Minute),
			25100+float64(i),
		))
	}

	snapshot := broker.Snapshot()
	if snapshot.ActiveSubscribers != 0 || snapshot.SlowSubscriberDrops != 1 {
		t.Fatalf("expected one slow-client eviction, got %+v", snapshot)
	}

	after := uint64(0)
	resumed, err := broker.Subscribe("NSE:NIFTY50", "1m", &after)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ResyncRequired {
		t.Fatal("recent slow client should be able to replay retained events")
	}
	if len(resumed.Replay) != 3 || resumed.Replay[2].Seq != 3 {
		t.Fatalf("unexpected retained replay: %+v", resumed.Replay)
	}
	resumed.Cancel()

	for i := 3; i < 12; i++ {
		broker.PublishBar(bar(
			"NSE:NIFTY50",
			"1m",
			base.Add(time.Duration(i)*time.Minute),
			25100+float64(i),
		))
	}
	staleResume, err := broker.Subscribe("NSE:NIFTY50", "1m", &after)
	if err != nil {
		t.Fatal(err)
	}
	if !staleResume.ResyncRequired {
		t.Fatal("resume older than replay retention must require REST/history resync")
	}
}

func TestWebSocketHundredClientFanout(t *testing.T) {
	broker := NewBroker(1024, 128)
	server := httptest.NewServer(NewWebSocketHandler(broker))
	defer server.Close()

	const clientCount = 100
	connections := make([]*websocket.Conn, 0, clientCount)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	for i := 0; i < clientCount; i++ {
		connection, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		connections = append(connections, connection)

		var message serverMessage
		if err := connection.ReadJSON(&message); err != nil {
			t.Fatalf("hello client %d: %v", i, err)
		}
		if message.Op != "hello" {
			t.Fatalf("client %d expected hello, got %+v", i, message)
		}
		if err := connection.WriteJSON(clientMessage{
			Op: "subscribe", Channel: "bars", Symbol: "NSE:NIFTY50", Timeframe: "15s",
		}); err != nil {
			t.Fatalf("subscribe client %d: %v", i, err)
		}
		if err := connection.ReadJSON(&message); err != nil {
			t.Fatalf("subscribed client %d: %v", i, err)
		}
		if message.Op != "subscribed" {
			t.Fatalf("client %d expected subscribed, got %+v", i, message)
		}
	}

	before := broker.Snapshot()
	if before.ActiveSubscribers != clientCount {
		t.Fatalf("active websocket subscribers=%d want %d", before.ActiveSubscribers, clientCount)
	}

	at := time.Date(2026, 9, 25, 3, 45, 0, 0, time.UTC)
	broker.PublishBar(bar("NSE:NIFTY50", "15s", at, 25123.5))

	for i, connection := range connections {
		if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatal(err)
		}
		var message serverMessage
		if err := connection.ReadJSON(&message); err != nil {
			t.Fatalf("read update client %d: %v", i, err)
		}
		if message.Op != "update" || message.Seq != 1 || message.Bar == nil || message.Bar.Close != 25123.5 {
			t.Fatalf("client %d unexpected update: %+v", i, message)
		}
	}

	after := broker.Snapshot()
	if after.SlowSubscriberDrops != 0 {
		t.Fatalf("100-client WebSocket fanout should not trigger backpressure drops: %+v", after)
	}
}
