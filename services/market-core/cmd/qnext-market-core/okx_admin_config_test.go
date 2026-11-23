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
