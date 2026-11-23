package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOKXGoldAdminSettingOverridesEnvironmentAndFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "q1-market.json")
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QNEXT_OKX_XAUUSD", "1")
	enabled, err := okxGoldRequested(path)
	if err != nil || !enabled {
		t.Fatalf("missing admin config must preserve env fallback: %v %v", enabled, err)
	}
	configPath := filepath.Join(filepath.Dir(path), "okx-xauusd.json")
	for _, test := range []struct {
		name, content string
		want          bool
		wantErr       bool
	}{
		{"disabled", `{"enabled":false}`, false, false},
		{"enabled", `{"enabled":true}`, true, false},
		{"non_boolean", `{"enabled":"true"}`, false, true},
		{"missing", `{}`, false, true},
		{"extra", `{"enabled":true,"command":"sudo"}`, false, true},
		{"multiple", `{"enabled":true}{"enabled":false}`, false, true},
		{"malformed", `{"enabled":`, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(configPath, []byte(test.content), 0640); err != nil {
				t.Fatal(err)
			}
			got, err := okxGoldRequested(path)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("enabled=%v err=%v want=%v error=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestOKXSwapAdminFlagIndependentOfXPerp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "q1-market.json")
	t.Setenv("QNEXT_OKX_XAUUSD", "1")
	t.Setenv("QNEXT_OKX_XAUUSDT", "0")
	if err := os.WriteFile(filepath.Join(dir, "okx-xauusd.json"), []byte(`{"enabled":true}`), 0640); err != nil {
		t.Fatal(err)
	}
	enabled, err := okxSwapRequested(path)
	if err != nil || enabled {
		t.Fatalf("XPerp setting must not enable USDT swap: %v, %v", enabled, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "okx-xauusdt.json"), []byte(`{"enabled":true}`), 0640); err != nil {
		t.Fatal(err)
	}
	enabled, err = okxSwapRequested(path)
	if err != nil || !enabled {
		t.Fatalf("swap must enable independently: %v, %v", enabled, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "okx-xauusdt.json"), []byte(`{"enabled":"true"}`), 0640); err != nil {
		t.Fatal(err)
	}
	enabled, err = okxSwapRequested(path)
	if err == nil || enabled {
		t.Fatalf("invalid swap setting must fail closed: %v %v", enabled, err)
	}
}
