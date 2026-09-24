package upstox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultIntradayBaseURL = "https://api.upstox.com/v3/historical-candle/intraday"

type ProviderCandle struct {
	OpenTime time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   float64
	OI       float64
}

type IntradayClient struct {
	HTTP    HTTPDoer
	BaseURL string
}

type intradayResponse struct {
	Status string `json:"status"`
	Data   struct {
		Candles []json.RawMessage `json:"candles"`
	} `json:"data"`
}

func (c IntradayClient) Fetch(
	ctx context.Context,
	accessToken string,
	instrumentKey string,
	timeframe string,
) ([]ProviderCandle, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("Upstox access token is required")
	}
	if strings.TrimSpace(instrumentKey) == "" {
		return nil, errors.New("Upstox instrument key is required")
	}
	interval, err := minuteInterval(timeframe)
	if err != nil {
		return nil, err
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultIntradayBaseURL
	}
	endpoint := base + "/" + url.PathEscape(instrumentKey) + "/minutes/" + strconv.Itoa(interval)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Upstox intraday candles: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("fetch Upstox intraday candles: unexpected HTTP status %d", response.StatusCode)
	}

	var payload intradayResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Upstox intraday candles: %w", err)
	}
	if payload.Status != "" && payload.Status != "success" {
		return nil, fmt.Errorf("fetch Upstox intraday candles: status %q", payload.Status)
	}

	candles := make([]ProviderCandle, 0, len(payload.Data.Candles))
	for _, raw := range payload.Data.Candles {
		candle, err := decodeProviderCandle(raw)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})
	return candles, nil
}

func decodeProviderCandle(raw json.RawMessage) (ProviderCandle, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return ProviderCandle{}, fmt.Errorf("decode Upstox candle row: %w", err)
	}
	if len(values) < 7 {
		return ProviderCandle{}, errors.New("Upstox candle row requires 7 values")
	}

	var timestamp string
	if err := json.Unmarshal(values[0], &timestamp); err != nil {
		return ProviderCandle{}, errors.New("invalid Upstox candle timestamp")
	}
	openTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return ProviderCandle{}, errors.New("invalid Upstox candle timestamp")
	}

	numbers := make([]float64, 6)
	for i := range numbers {
		if err := json.Unmarshal(values[i+1], &numbers[i]); err != nil {
			return ProviderCandle{}, fmt.Errorf("invalid Upstox candle numeric field %d", i+1)
		}
	}

	return ProviderCandle{
		OpenTime: openTime.UTC(),
		Open:     numbers[0],
		High:     numbers[1],
		Low:      numbers[2],
		Close:    numbers[3],
		Volume:   numbers[4],
		OI:       numbers[5],
	}, nil
}

func minuteInterval(timeframe string) (int, error) {
	switch timeframe {
	case "1m":
		return 1, nil
	case "3m":
		return 3, nil
	case "5m":
		return 5, nil
	default:
		return 0, fmt.Errorf("timeframe %q cannot be recovered from Upstox minute candles", timeframe)
	}
}
