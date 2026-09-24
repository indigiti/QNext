package main

import "testing"

func TestBuildRegistryExposesWorkspaceCatalogWithoutLiveConfig(t *testing.T) {
	registry, err := buildRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}

	visible := registry.ListVisible()
	if len(visible) != 2 {
		t.Fatalf("expected NIFTY and NIFTY-SYN workspace symbols, got %+v", visible)
	}
	if visible[0].Symbol != "NIFTY" || visible[0].CalendarID != "NSE_EQ" {
		t.Fatalf("unexpected NIFTY catalog entry: %+v", visible[0])
	}
	if visible[1].Symbol != "NIFTY-SYN" || !visible[1].Synthetic {
		t.Fatalf("unexpected synthetic catalog entry: %+v", visible[1])
	}
}
