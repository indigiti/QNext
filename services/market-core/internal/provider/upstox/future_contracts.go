package upstox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const DefaultInstrumentSearchURL = "https://api.upstox.com/v2/instruments/search"

type FutureContract struct {
	Name             string  `json:"name"`
	Segment          string  `json:"segment"`
	Exchange         string  `json:"exchange"`
	Expiry           string  `json:"expiry"`
	InstrumentKey    string  `json:"instrument_key"`
	ExchangeToken    string  `json:"exchange_token"`
	TradingSymbol    string  `json:"trading_symbol"`
	InstrumentType   string  `json:"instrument_type"`
	UnderlyingKey    string  `json:"underlying_key"`
	UnderlyingSymbol string  `json:"underlying_symbol"`
	LotSize          float64 `json:"lot_size,omitempty"`
}

type FutureContractSource interface {
	Futures(context.Context, string, string, string) ([]FutureContract, error)
}

type FutureContractsClient struct {
	HTTP     HTTPDoer
	Endpoint string
}

type futureContractsResponse struct {
	Status string           `json:"status"`
	Data   []FutureContract `json:"data"`
}

func (c FutureContractsClient) Futures(
	ctx context.Context,
	accessToken string,
	symbol string,
	exchange string,
) ([]FutureContract, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("Upstox access token is required")
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, errors.New("future search symbol is required")
	}
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	if exchange == "" {
		return nil, errors.New("future search exchange is required")
	}

	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultInstrumentSearchURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("query", symbol)
	query.Set("exchanges", exchange)
	query.Set("segments", "FUT")
	query.Set("instrument_types", "FUT")
	query.Set("expiry", "current_month,next_month")
	query.Set("page_number", "1")
	query.Set("records", "30")
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search Upstox futures: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("search Upstox futures: unexpected HTTP status %d", resp.StatusCode)
	}

	var payload futureContractsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Upstox futures: %w", err)
	}
	if payload.Status != "" && payload.Status != "success" {
		return nil, fmt.Errorf("search Upstox futures: status %q", payload.Status)
	}
	return payload.Data, nil
}
