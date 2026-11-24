package okx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const (
	ProviderName     = "okx"
	InstrumentID     = "OKX:XAUUSD-XPERP"
	SwapInstrumentID = "OKX:XAU-USDT-SWAP"
	defaultREST      = "https://www.okx.com"
	defaultWS        = "wss://ws.okx.com/ws/v5/business"
)

// Contract is discovered from the *live* OKX FUTURES instrument catalogue.
// Display labels and expired monthly futures are never treated as API IDs.
type Contract struct {
	InstrumentID  string // canonical QNeXT identity, independent of provider instId
	InstID        string
	ContractValue float64 // base-asset units per contract
	BaseCurrency  string  // empty preserves legacy XAU provenance
}

type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 12 * time.Second}
}
func (c Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultREST
}

type apiResponse[T any] struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []T    `json:"data"`
}
type instrumentInfo struct {
	InstID    string `json:"instId"`
	InstType  string `json:"instType"`
	State     string `json:"state"`
	CtVal     string `json:"ctVal"`
	CtValCcy  string `json:"ctValCcy"`
	CtType    string `json:"ctType"`
	SettleCcy string `json:"settleCcy"`
}

func (c Client) get(ctx context.Context, path string, query url.Values, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("okx %s: http %d", path, resp.StatusCode)
	}
	return json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 16<<20)).Decode(dst)
}

// DiscoverXAUUSD fails closed unless exactly one *live* gold X-Perp is returned.
// Never silently substitute a USDT swap, a price index, or an expiring future.
func (c Client) DiscoverXAUUSD(ctx context.Context) (Contract, error) {
	var response apiResponse[instrumentInfo]
	if err := c.get(ctx, "/api/v5/public/instruments", url.Values{"instType": {"FUTURES"}}, &response); err != nil {
		return Contract{}, err
	}
	if response.Code != "0" {
		return Contract{}, fmt.Errorf("okx instruments: %s (%s)", response.Code, response.Msg)
	}
	var found []Contract
	for _, item := range response.Data {
		id := strings.ToUpper(item.InstID)
		if item.InstType != "FUTURES" || item.State != "live" {
			continue
		}
		// OKX uses an expiry-suffixed ID for five-year X-Perps.
		// Ordinary dated gold futures never contain "_XPERP-".
		const prefix = "XAU-USD_UM_XPERP-"
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		expiry := strings.TrimPrefix(id, prefix)
		expiryDate, parseErr := time.Parse("060102", expiry)
		if parseErr != nil || len(expiry) != 6 || expiryDate.Before(time.Now().UTC()) {
			continue
		}
		// OKX reports ctValCcy for contract volume conversions.
		if item.CtValCcy != "XAU" {
			continue
		}
		v, err := strconv.ParseFloat(item.CtVal, 64)
		if err != nil || v <= 0 {
			continue
		}
		found = append(found, Contract{InstrumentID: InstrumentID, InstID: item.InstID, ContractValue: v})
	}
	if len(found) != 1 {
		return Contract{}, fmt.Errorf("okx expected one live XAUUSD X-Perp, found %d; refusing to guess an instrument ID", len(found))
	}
	return found[0], nil
}

const SwapProviderID = "XAU-USDT-SWAP"

// DiscoverXAUSwap uses the exact, live OKX linear USDT gold swap.
// It intentionally never aliases a USD expiry perpetual, mark index, or XAUT symbol.
// Contract size/currency must be learned from the live exchange metadata.
func (c Client) DiscoverXAUSwap(ctx context.Context) (Contract, error) {
	return c.DiscoverLinearSwap(ctx, SwapProviderID)
}

var linearSwapID = regexp.MustCompile(`^[A-Z0-9]{1,20}-[A-Z0-9]{1,20}-SWAP$`)

// DiscoverLinearSwap verifies an exact live contract and base-unit conversion.
// Inverse and quanto contracts are not supported by this normalized trade path.
func (c Client) DiscoverLinearSwap(ctx context.Context, providerKey string) (Contract, error) {
	if !linearSwapID.MatchString(providerKey) {
		return Contract{}, errors.New("invalid OKX linear swap ID")
	}
	parts := strings.Split(providerKey, "-")
	base, settle := parts[0], parts[1]
	var response apiResponse[instrumentInfo]
	q := url.Values{"instType": {"SWAP"}, "instId": {providerKey}}
	if err := c.get(ctx, "/api/v5/public/instruments", q, &response); err != nil {
		return Contract{}, err
	}
	if response.Code != "0" {
		return Contract{}, fmt.Errorf("okx swap instruments: %s (%s)", response.Code, response.Msg)
	}
	if len(response.Data) != 1 {
		return Contract{}, fmt.Errorf("okx swap: expected exact instrument, got %d", len(response.Data))
	}
	item := response.Data[0]
	if item.InstID != providerKey || item.InstType != "SWAP" || item.State != "live" {
		return Contract{}, errors.New("okx swap not live or mismatched instrument identity")
	}
	if item.CtType != "linear" || item.SettleCcy != settle || item.CtValCcy != base {
		return Contract{}, fmt.Errorf("okx swap unsupported contract units: ctType=%s settle=%s ctValCcy=%s",
			item.CtType, item.SettleCcy, item.CtValCcy)
	}
	v, err := strconv.ParseFloat(item.CtVal, 64)
	if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return Contract{}, errors.New("okx swap invalid contract value")
	}
	return Contract{InstrumentID: "OKX:" + providerKey, InstID: providerKey, ContractValue: v, BaseCurrency: base}, nil
}

func (c Contract) volumeSource() string {
	if c.BaseCurrency == "" || c.BaseCurrency == "XAU" {
		return "OKX_XAU_CONTRACT_TRADES"
	}
	return "OKX_" + c.BaseCurrency + "_CONTRACT_TRADES"
}

var barSize = map[string]string{
	"1s": "1s", "1m": "1m", "3m": "3m", "5m": "5m", "15m": "15m",
	"30m": "30m", "1h": "1H", "2h": "2H", "4h": "4H", "1D": "1Dutc",
}

func timeframeDuration(tf string) (time.Duration, error) {
	if tf == "5s" {
		return 5 * time.Second, nil
	}
	if tf == "1D" {
		return 24 * time.Hour, nil
	}
	if strings.HasSuffix(tf, "s") {
		n, e := strconv.Atoi(strings.TrimSuffix(tf, "s"))
		if e == nil && n > 0 {
			return time.Duration(n) * time.Second, nil
		}
	}
	if strings.HasSuffix(tf, "m") {
		n, e := strconv.Atoi(strings.TrimSuffix(tf, "m"))
		if e == nil && n > 0 {
			return time.Duration(n) * time.Minute, nil
		}
	}
	if strings.HasSuffix(tf, "h") {
		n, e := strconv.Atoi(strings.TrimSuffix(tf, "h"))
		if e == nil && n > 0 {
			return time.Duration(n) * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("unsupported okx timeframe %q", tf)
}

// History retrieves finalized exchange candles, paginating older records.
// OKX reports derivative 'vol' in contracts; QNext stores volume in XAU.
func (c Client) History(ctx context.Context, contract Contract, timeframe string, from, to time.Time) ([]domain.Bar, error) {
	if contract.InstID == "" || contract.ContractValue <= 0 {
		return nil, errors.New("verified OKX contract required")
	}
	if !from.Before(to) {
		return nil, errors.New("invalid OKX history range")
	}
	queryTF := timeframe
	if timeframe == "5s" {
		queryTF = "1s"
	}
	if _, ok := barSize[queryTF]; !ok {
		return nil, fmt.Errorf("unsupported OKX history timeframe: %s", timeframe)
	}
	duration, err := timeframeDuration(queryTF)
	if err != nil {
		return nil, err
	}
	cursor := to.UnixMilli()
	bars := make(map[int64]domain.Bar)
	canonicalID := contract.InstrumentID
	if canonicalID == "" {
		canonicalID = InstrumentID
	} // legacy X-Perp call sites
	for page := 0; page < 1000; page++ {
		q := url.Values{"instId": {contract.InstID}, "bar": {barSize[queryTF]}, "limit": {"300"}, "after": {strconv.FormatInt(cursor, 10)}}
		var response apiResponse[[]string]
		if err := c.get(ctx, "/api/v5/market/history-candles", q, &response); err != nil {
			return nil, err
		}
		if response.Code != "0" {
			return nil, fmt.Errorf("okx history: %s (%s)", response.Code, response.Msg)
		}
		if len(response.Data) == 0 {
			break
		}
		earliest := cursor
		for _, row := range response.Data {
			if len(row) < 9 || row[8] != "1" {
				continue
			} // never persist a forming candle
			ts, err := strconv.ParseInt(row[0], 10, 64)
			if err != nil {
				return nil, err
			}
			if ts >= cursor {
				continue
			}
			if ts < earliest {
				earliest = ts
			}
			openAt := time.UnixMilli(ts).UTC()
			if openAt.Before(from) || !openAt.Before(to) {
				continue
			}
			vals := make([]float64, 5)
			for i, index := range []int{1, 2, 3, 4, 5} {
				vals[i], err = strconv.ParseFloat(row[index], 64)
				if err != nil {
					return nil, err
				}
			}
			if vals[0] <= 0 || vals[1] < vals[0] || vals[2] > vals[0] || vals[1] < vals[3] || vals[2] > vals[3] || vals[4] < 0 {
				return nil, fmt.Errorf("invalid OKX OHLCV at %d", ts)
			}
			bars[ts] = domain.Bar{
				InstrumentID: canonicalID, Timeframe: queryTF, OpenTime: openAt, CloseTime: openAt.Add(duration),
				Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3],
				Volume: vals[4] * contract.ContractValue, VolumeSource: contract.volumeSource(), VolumeStatus: "OBSERVED",
				Final: true, AuthorityProvider: ProviderName, Quality: domain.QualityRecovered, Recovered: true,
				CandleEngineVersion: "okx-rest-v1", CreatedAt: time.Now().UTC(),
			}
		}
		if earliest >= cursor {
			break
		} // protects against a repeating pagination page
		cursor = earliest
		if cursor <= from.UnixMilli() {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	result := make([]domain.Bar, 0, len(bars))
	for _, bar := range bars {
		result = append(result, bar)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].OpenTime.Before(result[j].OpenTime) })
	if timeframe == "5s" {
		return fiveSecondFromOneSecond(result), nil
	}
	return result, nil
}

// Complete, contiguous one-second history is required to certify a recovered 5s
// bucket. Sparse or unavailable one-second data is left as a gap, not fabricated.
func fiveSecondFromOneSecond(source []domain.Bar) []domain.Bar {
	groups := make(map[int64][]domain.Bar)
	for _, b := range source {
		sec := b.OpenTime.Unix()
		start := sec - sec%5
		groups[start] = append(groups[start], b)
	}
	var result []domain.Bar
	for start, group := range groups {
		if len(group) != 5 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].OpenTime.Before(group[j].OpenTime) })
		contiguous := true
		for i, b := range group {
			if b.OpenTime.Unix() != start+int64(i) {
				contiguous = false
				break
			}
		}
		if !contiguous {
			continue
		}
		bar := group[0]
		bar.Timeframe = "5s"
		bar.OpenTime = time.Unix(start, 0).UTC()
		bar.CloseTime = bar.OpenTime.Add(5 * time.Second)
		bar.Close = group[4].Close
		bar.Volume = 0
		bar.CandleEngineVersion = "okx-1s-to-5s-v1"
		for _, b := range group {
			if b.High > bar.High {
				bar.High = b.High
			}
			if b.Low < bar.Low {
				bar.Low = b.Low
			}
			bar.Volume += b.Volume
		}
		result = append(result, bar)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].OpenTime.Before(result[j].OpenTime) })
	return result
}
