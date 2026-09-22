package main

import (
	"maps"
	"slices"
	"testing"
)

// Pins the semantics the hand-rolled isDataEqual had: it decides where one
// availability block ends and the next begins.
func TestAvailabilityDataEquality(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b map[string][]string
		want bool
	}{
		{"identical", map[string][]string{"binance": {"btc_usdt", "eth_usdt"}}, map[string][]string{"binance": {"btc_usdt", "eth_usdt"}}, true},
		{"both empty", map[string][]string{}, map[string][]string{}, true},
		{"extra exchange", map[string][]string{"binance": {"btc_usdt"}}, map[string][]string{"binance": {"btc_usdt"}, "okx": {"btc_usdt"}}, false},
		{"renamed exchange", map[string][]string{"binance": {"btc_usdt"}}, map[string][]string{"okx": {"btc_usdt"}}, false},
		{"extra pair", map[string][]string{"binance": {"btc_usdt"}}, map[string][]string{"binance": {"btc_usdt", "eth_usdt"}}, false},
		{"different pair", map[string][]string{"binance": {"btc_usdt"}}, map[string][]string{"binance": {"eth_usdt"}}, false},
		{"order matters", map[string][]string{"binance": {"btc_usdt", "eth_usdt"}}, map[string][]string{"binance": {"eth_usdt", "btc_usdt"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := maps.EqualFunc(tc.a, tc.b, slices.Equal); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
