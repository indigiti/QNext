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

const DefaultAuthorizeURL = "https://api.upstox.com/v3/feed/market-data-feed/authorize"

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Authorizer struct {
	HTTP     HTTPDoer
	Endpoint string
}

type authorizeResponse struct {
	Status string `json:"status"`
	Data   struct {
		AuthorizedRedirectURI string `json:"authorized_redirect_uri"`
	} `json:"data"`
}

func (a Authorizer) AuthorizedURI(ctx context.Context, accessToken string) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("Upstox access token is required")
	}

	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := a.Endpoint
	if endpoint == "" {
		endpoint = DefaultAuthorizeURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("authorize market feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return "", fmt.Errorf("authorize market feed: unexpected HTTP status %d", resp.StatusCode)
	}

	var payload authorizeResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("decode market-feed authorization response: %w", err)
	}
	if payload.Status != "" && payload.Status != "success" {
		return "", fmt.Errorf("authorize market feed: status %q", payload.Status)
	}

	rawURI := strings.TrimSpace(payload.Data.AuthorizedRedirectURI)
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "wss" || parsed.Host == "" {
		return "", errors.New("authorize market feed: invalid websocket URI")
	}
	return rawURI, nil
}
