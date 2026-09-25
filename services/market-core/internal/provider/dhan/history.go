package dhan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
	"github.com/indigiti/QNext/services/market-core/internal/symbol"
)

const defaultHistoryURL = "https://api.dhan.co/v2/charts/intraday"

type ProviderCandle struct {
	OpenTime                       time.Time
	Open, High, Low, Close, Volume float64
}
type HistoricalFetcher interface {
	Fetch(context.Context, string, InstrumentKey, string, time.Time, time.Time) ([]ProviderCandle, error)
}
type HistoryClient struct {
	URL        string
	HTTPClient *http.Client
}
type historyResponse struct {
	Open      []float64 `json:"open"`
	High      []float64 `json:"high"`
	Low       []float64 `json:"low"`
	Close     []float64 `json:"close"`
	Volume    []float64 `json:"volume"`
	Timestamp []int64   `json:"timestamp"`
}

func (c HistoryClient) Fetch(ctx context.Context, token string, key InstrumentKey, timeframe string, from, to time.Time) ([]ProviderCandle, error) {
	interval := ""
	aggregate := time.Duration(0)
	switch timeframe {
	case "1m":
		interval = "1"
	case "3m":
		interval = "1"
		aggregate = 3 * time.Minute
	case "5m":
		interval = "5"
	default:
		return nil, fmt.Errorf("unsupported Dhan recovery timeframe %q", timeframe)
	}
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return nil, err
	}
	body := map[string]any{"securityId": fmt.Sprint(key.SecurityID), "exchangeSegment": key.ExchangeSegment, "instrument": key.Instrument, "interval": interval, "oi": false, "fromDate": from.In(loc).Format("2006-01-02 15:04:05"), "toDate": to.In(loc).Format("2006-01-02 15:04:05")}
	payload, _ := json.Marshal(body)
	url := c.URL
	if url == "" {
		url = defaultHistoryURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access-token", token)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Dhan intraday history request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Dhan intraday history HTTP %d", resp.StatusCode)
	}
	var decoded historyResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	n := len(decoded.Timestamp)
	if len(decoded.Open) != n || len(decoded.High) != n || len(decoded.Low) != n || len(decoded.Close) != n || len(decoded.Volume) != n {
		return nil, errors.New("Dhan intraday history arrays have inconsistent lengths")
	}
	raw := make([]ProviderCandle, 0, n)
	for i := 0; i < n; i++ {
		raw = append(raw, ProviderCandle{OpenTime: time.Unix(decoded.Timestamp[i], 0).UTC(), Open: decoded.Open[i], High: decoded.High[i], Low: decoded.Low[i], Close: decoded.Close[i], Volume: decoded.Volume[i]})
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].OpenTime.Before(raw[j].OpenTime) })
	if aggregate > 0 {
		return aggregateCandles(raw, aggregate), nil
	}
	return raw, nil
}
func aggregateCandles(in []ProviderCandle, d time.Duration) []ProviderCandle {
	if len(in) == 0 {
		return nil
	}
	by := map[time.Time]ProviderCandle{}
	order := []time.Time{}
	for _, c := range in {
		bucket := c.OpenTime.UTC().Truncate(d)
		v, ok := by[bucket]
		if !ok {
			v = c
			v.OpenTime = bucket
			order = append(order, bucket)
		} else {
			if c.High > v.High {
				v.High = c.High
			}
			if c.Low < v.Low {
				v.Low = c.Low
			}
			v.Close = c.Close
			v.Volume += c.Volume
		}
		by[bucket] = v
	}
	sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
	out := make([]ProviderCandle, 0, len(order))
	for _, k := range order {
		out = append(out, by[k])
	}
	return out
}

type HistoryWriter interface{ AppendBar(domain.Bar) error }
type Recovery struct {
	Client      HistoricalFetcher
	AccessToken string
	Registry    *symbol.Registry
	History     HistoryWriter
	Timeframes  []string
}

func (r *Recovery) Recover(ctx context.Context, instrumentID string, from, to time.Time) error {
	if r.Client == nil || r.Registry == nil || r.History == nil {
		return errors.New("Dhan recovery requires client, registry, and history")
	}
	mapping, ok := r.Registry.ProviderMapping(ProviderName, instrumentID)
	if !ok {
		return errors.New("Dhan recovery mapping is not registered")
	}
	key, err := ParseInstrumentKey(mapping.ProviderKey)
	if err != nil {
		return err
	}
	frames := r.Timeframes
	if len(frames) == 0 {
		frames = []string{"1m", "3m", "5m"}
	}
	for _, tf := range frames {
		candles, err := r.Client.Fetch(ctx, r.AccessToken, key, tf, from, to)
		if err != nil {
			return err
		}
		dur := time.Minute
		if tf == "3m" {
			dur = 3 * time.Minute
		} else if tf == "5m" {
			dur = 5 * time.Minute
		}
		for _, c := range candles {
			closeTime := c.OpenTime.Add(dur)
			if !closeTime.After(from) || closeTime.After(to) {
				continue
			}
			bar := domain.Bar{InstrumentID: instrumentID, Timeframe: tf, OpenTime: c.OpenTime, CloseTime: closeTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Final: true, AuthorityProvider: ProviderName, Quality: domain.QualityRecovered, Recovered: true, CandleEngineVersion: "provider-recovery-v1"}
			if err := r.History.AppendBar(bar); err != nil {
				return err
			}
		}
	}
	return nil
}
