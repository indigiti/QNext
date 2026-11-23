package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// okxGoldRequested allows Admin → Market Data to persist the gold setting
// without writing secrets, editing systemd units, or changing index config.
// When the admin file exists it takes precedence over the legacy env flag.
func okxGoldRequested(marketConfigPath string) (bool, error) {
	fallback := strings.TrimSpace(os.Getenv("QNEXT_OKX_XAUUSD")) == "1"
	if strings.TrimSpace(marketConfigPath) == "" {
		return fallback, nil
	}
	path := filepath.Join(filepath.Dir(marketConfigPath), "okx-xauusd.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fallback, nil
	}
	if err != nil {
		return false, fmt.Errorf("read OKX gold admin config: %w", err)
	}
	var setting struct {
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&setting); err != nil {
		return false, fmt.Errorf("invalid OKX gold admin config: %w", err)
	}
	if setting.Enabled == nil {
		return false, errors.New("OKX gold config requires boolean enabled")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return false, errors.New("OKX gold config must contain exactly one JSON object")
	}
	return *setting.Enabled, nil
}
