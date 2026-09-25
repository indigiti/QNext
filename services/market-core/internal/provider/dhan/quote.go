package dhan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultQuoteURL = "https://api.dhan.co/v2/marketfeed/ltp"

type QuoteSnapshot struct {
	ProviderKey string
	Price       float64
	ObservedAt  time.Time
}
type QuoteFetcher interface {
	Fetch(context.Context, string, string, []InstrumentKey) ([]QuoteSnapshot, error)
}
type QuoteClient struct {
	URL        string
	HTTPClient *http.Client
	Now        func() time.Time
}
type quoteResponse struct {
	Status string `json:"status"`
	Data   map[string]map[string]struct {
		LastPrice float64 `json:"last_price"`
	} `json:"data"`
}

func (c QuoteClient) Fetch(ctx context.Context, accessToken, clientID string, keys []InstrumentKey) ([]QuoteSnapshot, error) {
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(clientID) == "" {
		return nil, errors.New("Dhan access token and client id are required")
	}
	if len(keys) == 0 {
		return nil, errors.New("at least one Dhan instrument is required")
	}
	if len(keys) > 1000 {
		return nil, errors.New("Dhan market quote supports at most 1000 instruments per request")
	}
	body := map[string][]int64{}
	lookup := map[string]InstrumentKey{}
	for _, k := range keys {
		body[k.ExchangeSegment] = append(body[k.ExchangeSegment], k.SecurityID)
		lookup[k.ExchangeSegment+"|"+fmt.Sprint(k.SecurityID)] = k
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := c.URL
	if url == "" {
		url = defaultQuoteURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access-token", accessToken)
	req.Header.Set("client-id", clientID)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Dhan market quote request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Dhan market quote HTTP %d", resp.StatusCode)
	}
	var decoded quoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode Dhan market quote: %w", err)
	}
	if decoded.Status != "" && !strings.EqualFold(decoded.Status, "success") {
		return nil, errors.New("Dhan market quote returned non-success status")
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	observed := now().UTC()
	out := make([]QuoteSnapshot, 0, len(keys))
	for segment, byID := range decoded.Data {
		for id, row := range byID {
			k, ok := lookup[segment+"|"+id]
			if !ok || row.LastPrice <= 0 {
				continue
			}
			out = append(out, QuoteSnapshot{ProviderKey: k.String(), Price: row.LastPrice, ObservedAt: observed})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("Dhan market quote returned no configured instruments")
	}
	return out, nil
}
