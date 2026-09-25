package dhan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultOptionChainURL = "https://api.dhan.co/v2/optionchain"

type OptionChainLeg struct {
	Strike     float64
	Side       string
	SecurityID int64
}

type OptionChainSource interface {
	Chain(context.Context, string, string, InstrumentKey, string) ([]OptionChainLeg, error)
}

type OptionChainClient struct {
	URL        string
	HTTPClient *http.Client
}

type optionChainResponse struct {
	Status string `json:"status"`
	Data   struct {
		OC map[string]struct {
			CE *struct {
				SecurityID int64 `json:"security_id"`
			} `json:"ce"`
			PE *struct {
				SecurityID int64 `json:"security_id"`
			} `json:"pe"`
		} `json:"oc"`
	} `json:"data"`
}

func (c OptionChainClient) Chain(
	ctx context.Context,
	accessToken string,
	clientID string,
	underlying InstrumentKey,
	expiry string,
) ([]OptionChainLeg, error) {
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(clientID) == "" {
		return nil, errors.New("Dhan access token and client id are required")
	}
	if underlying.SecurityID <= 0 || underlying.ExchangeSegment == "" {
		return nil, errors.New("Dhan option chain requires an underlying instrument")
	}
	if strings.TrimSpace(expiry) == "" {
		return nil, errors.New("Dhan option chain requires expiry")
	}

	payload, err := json.Marshal(map[string]any{
		"UnderlyingScrip": underlying.SecurityID,
		"UnderlyingSeg":   underlying.ExchangeSegment,
		"Expiry":          expiry,
	})
	if err != nil {
		return nil, err
	}
	url := c.URL
	if url == "" {
		url = defaultOptionChainURL
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
		return nil, fmt.Errorf("Dhan option chain request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Dhan option chain HTTP %d", resp.StatusCode)
	}

	var decoded optionChainResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode Dhan option chain: %w", err)
	}
	if decoded.Status != "" && !strings.EqualFold(decoded.Status, "success") {
		return nil, errors.New("Dhan option chain returned non-success status")
	}

	result := make([]OptionChainLeg, 0, len(decoded.Data.OC)*2)
	for rawStrike, pair := range decoded.Data.OC {
		strike, err := strconv.ParseFloat(strings.TrimSpace(rawStrike), 64)
		if err != nil {
			continue
		}
		if pair.CE != nil && pair.CE.SecurityID > 0 {
			result = append(result, OptionChainLeg{Strike: strike, Side: "CE", SecurityID: pair.CE.SecurityID})
		}
		if pair.PE != nil && pair.PE.SecurityID > 0 {
			result = append(result, OptionChainLeg{Strike: strike, Side: "PE", SecurityID: pair.PE.SecurityID})
		}
	}
	if len(result) == 0 {
		return nil, errors.New("Dhan option chain returned no option security ids")
	}
	return result, nil
}
