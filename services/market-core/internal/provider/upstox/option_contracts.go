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

const DefaultOptionContractsURL = "https://api.upstox.com/v2/option/contract"

type OptionContract struct {
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
	StrikePrice      float64 `json:"strike_price"`
	Weekly           bool    `json:"weekly"`
}

type OptionContractSource interface {
	Contracts(context.Context, string, string) ([]OptionContract, error)
}

type OptionContractsClient struct {
	HTTP     HTTPDoer
	Endpoint string
}

type optionContractsResponse struct {
	Status string           `json:"status"`
	Data   []OptionContract `json:"data"`
}

func (c OptionContractsClient) Contracts(
	ctx context.Context,
	accessToken string,
	underlyingKey string,
) ([]OptionContract, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("Upstox access token is required")
	}
	if strings.TrimSpace(underlyingKey) == "" {
		return nil, errors.New("underlying instrument key is required")
	}

	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultOptionContractsURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("instrument_key", underlyingKey)
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
		return nil, fmt.Errorf("fetch Upstox option contracts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("fetch Upstox option contracts: unexpected HTTP status %d", resp.StatusCode)
	}

	var payload optionContractsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Upstox option contracts: %w", err)
	}
	if payload.Status != "" && payload.Status != "success" {
		return nil, fmt.Errorf("fetch Upstox option contracts: status %q", payload.Status)
	}
	return payload.Data, nil
}
