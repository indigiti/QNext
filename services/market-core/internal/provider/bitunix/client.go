// Package bitunix implements public, read-only crypto futures market data.
package bitunix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const ProviderName = "bitunix"

type Contract struct {
	Symbol string
	Base   string
	Quote  string
}

func (c Contract) ID() string { return "BITUNIX:" + c.Symbol }

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type response struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

func (c Client) get(ctx context.Context, path string, query url.Values, out any) error {
	base := c.BaseURL
	if base == "" {
		base = "https://fapi.bitunix.com"
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Bitunix HTTP %d", res.StatusCode)
	}
	var envelope response
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 4<<20)).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Code != 0 || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("Bitunix response rejected: %d", envelope.Code)
	}
	return json.Unmarshal(envelope.Data, out)
}

// Only the initial BTC/ETH USDT crypto lane is certified by fixtures.
// Other instruments need an explicit market/calendar review first.
func Supported(symbol string) bool {
	return symbol == "BTCUSDT" || symbol == "ETHUSDT"
}

func (c Client) Discover(ctx context.Context, symbol string) (Contract, error) {
	if !Supported(symbol) {
		return Contract{}, errors.New("Bitunix symbol outside initial crypto lane")
	}
	var rows []struct {
		Symbol string `json:"symbol"`
		Base   string `json:"base"`
		Quote  string `json:"quote"`
		Status string `json:"symbolStatus"`
	}
	if err := c.get(ctx, "/api/v1/futures/market/trading_pairs", url.Values{"symbols": {symbol}}, &rows); err != nil {
		return Contract{}, err
	}
	if len(rows) != 1 || rows[0].Symbol != symbol || rows[0].Status != "OPEN" ||
		rows[0].Quote != "USDT" || rows[0].Base+rows[0].Quote != symbol {
		return Contract{}, errors.New("Bitunix exact open contract not verified")
	}
	return Contract{Symbol: symbol, Base: rows[0].Base, Quote: rows[0].Quote}, nil
}

func number(raw json.RawMessage) (float64, error) {
	s := strings.Trim(string(raw), "\"")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, errors.New("Bitunix invalid numeric field")
	}
	return n, nil
}

type candleRow struct {
	Open  json.RawMessage `json:"open"`
	High  json.RawMessage `json:"high"`
	Low   json.RawMessage `json:"low"`
	Close json.RawMessage `json:"close"`
	Base  json.RawMessage `json:"baseVol"`
	Quote json.RawMessage `json:"quoteVol"`
	Time  int64           `json:"time"`
	Type  string          `json:"type"`
}

func normalizeBar(c Contract, row candleRow) (domain.Bar, error) {
	vals := make([]float64, 6)
	for i, raw := range []json.RawMessage{row.Open, row.High, row.Low, row.Close, row.Base, row.Quote} {
		v, err := number(raw)
		if err != nil {
			return domain.Bar{}, err
		}
		vals[i] = v
	}
	o, h, l, closePrice, base, quote := vals[0], vals[1], vals[2], vals[3], vals[4], vals[5]
	if row.Time <= 0 || row.Time%60000 != 0 || row.Type != "LAST_PRICE" ||
		l <= 0 || h < l || o < l || o > h || closePrice < l || closePrice > h || base < 0 || quote < 0 {
		return domain.Bar{}, errors.New("Bitunix invalid closed market candle")
	}
	// Docs label baseVol as base units, but the published example reverses
	// apparent units. Require turnover/volume to agree with OHLC; never guess.
	if (base == 0 && quote != 0) || (base > 0 && (quote <= 0 || quote/base < l*0.99 || quote/base > h*1.01)) {
		return domain.Bar{}, errors.New("Bitunix ambiguous historical volume units")
	}
	start := time.UnixMilli(row.Time).UTC()
	return domain.Bar{InstrumentID: c.ID(), Timeframe: "1m", OpenTime: start, CloseTime: start.Add(time.Minute),
		Open: o, High: h, Low: l, Close: closePrice, Volume: base,
		VolumeSource: "BITUNIX_BASE_TRADES", VolumeStatus: "OBSERVED", AuthorityProvider: ProviderName,
		Final: true, Quality: domain.QualityRecovered, Recovered: true, CandleEngineVersion: "bitunix-rest-v1"}, nil
}

// History requests only closed last-price 1m bars; never synthesize 5s history.
func (c Client) History(ctx context.Context, contract Contract, tf string, from, to time.Time) ([]domain.Bar, error) {
	if tf != "1m" || !Supported(contract.Symbol) || !from.Before(to) {
		return nil, errors.New("Bitunix requires 1m closed-history range")
	}
	end := to.UTC().Truncate(time.Minute)
	now := time.Now().UTC().Truncate(time.Minute)
	if end.After(now) {
		end = now
	}
	cursor := end.UnixMilli()
	out := make([]domain.Bar, 0, 1440)
	seen := make(map[int64]bool)
	for page := 0; page < 16 && cursor > from.UnixMilli(); page++ {
		if page > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
		var rows []candleRow
		q := url.Values{"symbol": {contract.Symbol}, "interval": {"1m"}, "type": {"LAST_PRICE"},
			"endTime": {strconv.FormatInt(cursor, 10)}, "limit": {"200"}}
		if err := c.get(ctx, "/api/v1/futures/market/kline", q, &rows); err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		earliest := cursor
		for _, row := range rows {
			if row.Time >= cursor {
				return nil, errors.New("Bitunix non-exclusive history cursor")
			}
			if row.Time < earliest {
				earliest = row.Time
			}
			if row.Time < from.UnixMilli() || seen[row.Time] {
				continue
			}
			bar, err := normalizeBar(contract, row)
			if err != nil {
				return nil, err
			}
			if bar.CloseTime.After(end) {
				return nil, errors.New("Bitunix returned forming candle")
			}
			seen[row.Time] = true
			out = append(out, bar)
		}
		cursor = earliest
		if len(rows) < 200 {
			break
		}
		if page == 15 && cursor > from.UnixMilli() {
			return nil, errors.New("Bitunix history pagination limit")
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenTime.Before(out[j].OpenTime) })
	return out, nil
}
