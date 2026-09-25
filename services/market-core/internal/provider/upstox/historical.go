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
	"strings"
	"time"
)

const DefaultHistoricalBaseURL = "https://api.upstox.com/v3/historical-candle"

type HistoricalRangeClient struct {
	HTTP            HTTPDoer
	BaseURL         string
	Intraday        IntradayFetcher
	Now             func() time.Time
	MarketTimezone  string
}

func (c HistoricalRangeClient) FetchRange(
	ctx context.Context,
	accessToken string,
	instrumentKey string,
	from time.Time,
	to time.Time,
) ([]ProviderCandle, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("Upstox access token is required")
	}
	if strings.TrimSpace(instrumentKey) == "" {
		return nil, errors.New("Upstox instrument key is required")
	}
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, errors.New("invalid historical candle range")
	}
	if to.Sub(from) > 31*24*time.Hour {
		return nil, errors.New("historical candle range cannot exceed 31 days")
	}

	timezone := strings.TrimSpace(c.MarketTimezone)
	if timezone == "" {
		timezone = "Asia/Kolkata"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load market timezone: %w", err)
	}

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	current := now().UTC()
	if to.After(current) {
		to = current
	}
	if !from.Before(to) {
		return nil, nil
	}

	localNow := current.In(location)
	todayStart := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		0, 0, 0, 0, location,
	)
	localFrom := from.In(location)
	localTo := to.In(location)

	byOpen := make(map[int64]ProviderCandle)

	if localFrom.Before(todayStart) {
		historicalTo := localTo
		if !historicalTo.Before(todayStart) {
			historicalTo = todayStart.Add(-time.Nanosecond)
		}
		if historicalTo.After(localFrom) {
			candles, err := c.fetchHistoricalUnit(
				ctx,
				accessToken,
				instrumentKey,
				"minutes",
				1,
				localFrom.Format("2006-01-02"),
				historicalTo.Format("2006-01-02"),
			)
			if err != nil {
				return nil, err
			}
			for _, candle := range candles {
				if !candle.OpenTime.Before(from) && candle.OpenTime.Before(to) {
					byOpen[candle.OpenTime.UnixMilli()] = candle
				}
			}
		}
	}

	if localTo.After(todayStart) {
		intraday := c.Intraday
		if intraday == nil {
			intraday = IntradayClient{}
		}
		candles, err := intraday.Fetch(ctx, accessToken, instrumentKey, "1m")
		if err != nil {
			return nil, err
		}
		for _, candle := range candles {
			if !candle.OpenTime.Before(from) &&
				candle.OpenTime.Before(to) &&
				!candle.OpenTime.Add(time.Minute).After(current) {
				byOpen[candle.OpenTime.UnixMilli()] = candle
			}
		}
	}

	result := make([]ProviderCandle, 0, len(byOpen))
	for _, candle := range byOpen {
		result = append(result, candle)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].OpenTime.Before(result[j].OpenTime)
	})
	return result, nil
}

func (c HistoricalRangeClient) FetchMonthlyRange(
	ctx context.Context,
	accessToken string,
	instrumentKey string,
	from time.Time,
	to time.Time,
) ([]ProviderCandle, error) {
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, errors.New("invalid monthly historical range")
	}
	return c.fetchHistoricalUnit(
		ctx,
		accessToken,
		instrumentKey,
		"months",
		1,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
	)
}

func (c HistoricalRangeClient) fetchHistoricalUnit(
	ctx context.Context,
	accessToken string,
	instrumentKey string,
	unit string,
	interval int,
	fromDate string,
	toDate string,
) ([]ProviderCandle, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultHistoricalBaseURL
	}

	endpoint := base + "/" +
		url.PathEscape(instrumentKey) + "/" +
		url.PathEscape(unit) + "/" +
		strconv.Itoa(interval) + "/" +
		url.PathEscape(toDate) + "/" +
		url.PathEscape(fromDate)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Upstox historical candles: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf(
			"fetch Upstox historical candles: unexpected HTTP status %d",
			response.StatusCode,
		)
	}

	var payload intradayResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Upstox historical candles: %w", err)
	}
	if payload.Status != "" && payload.Status != "success" {
		return nil, fmt.Errorf("fetch Upstox historical candles: status %q", payload.Status)
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
