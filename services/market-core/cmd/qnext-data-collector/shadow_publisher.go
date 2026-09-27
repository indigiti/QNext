package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/research"
)

const researchBridgeTokenHeader = "X-QNext-Research-Token"

type synPlusShadowPublisher struct {
	client   *http.Client
	endpoint string
	token    string
	queue    chan research.SynPlusSnapshot
	once     sync.Once
	wg       sync.WaitGroup
}

func newSynPlusShadowPublisher(baseURL, accessToken string) *synPlusShadowPublisher {
	publisher := &synPlusShadowPublisher{
		client: &http.Client{
			Timeout: 800 * time.Millisecond,
		},
		endpoint: strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/api/v1/stream",
		token:    researchBridgeToken(accessToken),
		queue:    make(chan research.SynPlusSnapshot, 1),
	}
	publisher.wg.Add(1)
	go publisher.run()
	return publisher
}

func (p *synPlusShadowPublisher) Publish(snapshot research.SynPlusSnapshot) {
	if p == nil {
		return
	}
	select {
	case p.queue <- snapshot:
		return
	default:
	}

	// The durable research record and chart history are already persisted by
	// SynPlusCollector. For live fan-out, retain only the newest snapshot when
	// Market Core is briefly slower or restarting.
	select {
	case <-p.queue:
	default:
	}
	select {
	case p.queue <- snapshot:
	default:
	}
}

func (p *synPlusShadowPublisher) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		close(p.queue)
		p.wg.Wait()
	})
}

func (p *synPlusShadowPublisher) run() {
	defer p.wg.Done()
	for snapshot := range p.queue {
		if err := p.post(snapshot); err != nil {
			log.Printf("SYN+ live shadow publish: %v", err)
		}
	}
}

func (p *synPlusShadowPublisher) post(snapshot research.SynPlusSnapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(researchBridgeTokenHeader, p.token)

	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &shadowPublishHTTPError{status: response.StatusCode}
	}
	return nil
}

type shadowPublishHTTPError struct {
	status int
}

func (e *shadowPublishHTTPError) Error() string {
	return http.StatusText(e.status)
}

func researchBridgeToken(accessToken string) string {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("qnext-syn-plus-shadow:" + accessToken))
	return hex.EncodeToString(sum[:])
}
