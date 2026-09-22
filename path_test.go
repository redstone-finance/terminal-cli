package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLocalPathSanitizesReservedChars(t *testing.T) {
	date, err := time.Parse("2006-01-02", "2026-01-09")
	if err != nil {
		t.Fatal(err)
	}
	rel := getRelativePath("hyna", "perp:hyna:btc_usd", "derivative", date)

	want := "hyna/derivative/2026/01/09/perp:hyna:btc_usd/hyna_derivative_2026-01-09_perp:hyna:btc_usd.parquet"
	if rel != want {
		t.Fatalf("the API path must keep its colons:\n got %s\nwant %s", rel, want)
	}

	wantLocal := filepath.Join("downloads", "hyna", "derivative", "2026", "01", "09",
		"perp_hyna_btc_usd", "hyna_derivative_2026-01-09_perp_hyna_btc_usd.parquet")
	if got := localPath(rel); got != wantLocal {
		t.Errorf("got  %s\nwant %s", got, wantLocal)
	}
}
