package stream

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketLoadCertification(t *testing.T) {
	if os.Getenv("QNEXT_WSS_LOAD_CERT") != "1" {
		t.Skip("set QNEXT_WSS_LOAD_CERT=1 to run websocket load certification")
	}

	for _, clients := range []int{100, 250, 500, 1000} {
		clients := clients
		t.Run(fmt.Sprintf("clients_%d", clients), func(t *testing.T) {
			certifyWebSocketFanout(t, clients, 10)
		})
	}
}

type loadClient struct {
	connection *websocket.Conn
}

func certifyWebSocketFanout(t *testing.T, clientCount, subscriptionsPerClient int) {
	t.Helper()
	broker := NewBroker(128, 16)
	server := httptest.NewServer(NewWebSocketHandler(broker))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	started := time.Now()
	clients := make([]loadClient, clientCount)
	errors := make(chan error, clientCount)
	sem := make(chan struct{}, 64)
	var wg sync.WaitGroup

	for index := 0; index < clientCount; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				errors <- fmt.Errorf("client %d dial: %w", index, err)
				return
			}
			_ = connection.SetReadDeadline(time.Now().Add(30 * time.Second))
			var hello serverMessage
			if err := connection.ReadJSON(&hello); err != nil || hello.Op != "hello" {
				connection.Close()
				if err == nil {
					err = fmt.Errorf("unexpected hello: %+v", hello)
				}
				errors <- fmt.Errorf("client %d hello: %w", index, err)
				return
			}

			for streamIndex := 0; streamIndex < subscriptionsPerClient; streamIndex++ {
				instrument := fmt.Sprintf("LOAD:INDEX:%02d", streamIndex)
				if err := connection.WriteJSON(clientMessage{
					Op:        "subscribe",
					Channel:   "bars",
					Symbol:    instrument,
					Timeframe: "15s",
				}); err != nil {
					connection.Close()
					errors <- fmt.Errorf("client %d subscribe %d: %w", index, streamIndex, err)
					return
				}
				var ack serverMessage
				if err := connection.ReadJSON(&ack); err != nil || ack.Op != "subscribed" {
					connection.Close()
					if err == nil {
						err = fmt.Errorf("unexpected ack: %+v", ack)
					}
					errors <- fmt.Errorf("client %d ack %d: %w", index, streamIndex, err)
					return
				}
			}
			clients[index] = loadClient{connection: connection}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, client := range clients {
		defer client.connection.Close()
	}

	expectedSubscribers := clientCount * subscriptionsPerClient
	stats := broker.Stats()
	if stats.Subscribers != expectedSubscribers {
		t.Fatalf("unexpected broker subscribers: got %d want %d", stats.Subscribers, expectedSubscribers)
	}

	readErrors := make(chan error, clientCount)
	var readers sync.WaitGroup
	for index, client := range clients {
		index := index
		connection := client.connection
		readers.Add(1)
		go func() {
			defer readers.Done()
			finals := make(map[string]bool, subscriptionsPerClient)
			_ = connection.SetReadDeadline(time.Now().Add(30 * time.Second))
			for len(finals) < subscriptionsPerClient {
				var message serverMessage
				if err := connection.ReadJSON(&message); err != nil {
					readErrors <- fmt.Errorf("client %d read: %w", index, err)
					return
				}
				if message.Op == "update" && message.Bar != nil && message.Bar.Final {
					finals[message.StreamID] = true
				}
			}
		}()
	}

	base := time.Date(2026, 9, 30, 9, 15, 0, 0, time.UTC)
	publishStarted := time.Now()
	for streamIndex := 0; streamIndex < subscriptionsPerClient; streamIndex++ {
		instrument := fmt.Sprintf("LOAD:INDEX:%02d", streamIndex)
		for revision := uint32(1); revision <= 4; revision++ {
			forming := bar(instrument, "15s", base, 25000+float64(streamIndex)+float64(revision)/10)
			forming.Revision = revision
			broker.PublishBar(forming)
		}
		final := bar(instrument, "15s", base, 25000+float64(streamIndex)+0.5)
		final.Final = true
		final.Revision = 5
		broker.PublishBar(final)
	}
	publishDuration := time.Since(publishStarted)

	readers.Wait()
	close(readErrors)
	for err := range readErrors {
		if err != nil {
			t.Fatal(err)
		}
	}

	stats = broker.Stats()
	if stats.Subscribers != expectedSubscribers {
		t.Fatalf("fanout lost subscribers: got %d want %d; stats=%+v", stats.Subscribers, expectedSubscribers, stats)
	}
	if stats.SlowDisconnects != 0 {
		t.Fatalf("load certification produced slow-client disconnects: %+v", stats)
	}

	t.Logf(
		"certified clients=%d logical_subscriptions=%d publish=%s end_to_end=%s coalesced=%d dropped_forming=%d",
		clientCount,
		expectedSubscribers,
		publishDuration.Round(time.Millisecond),
		time.Since(started).Round(time.Millisecond),
		stats.CoalescedForming,
		stats.DroppedForming,
	)
}
