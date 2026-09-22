package main

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{"trims pflag's leading spaces", []string{"binance", " bybit"}, []string{"binance", "bybit"}},
		{"drops duplicates", []string{"btc_usdt", "btc_usdt"}, []string{"btc_usdt"}},
		{"drops empties", []string{"", " ", "okx"}, []string{"okx"}},
		{"keeps order", []string{"c", "a", "b", "a"}, []string{"c", "a", "b"}},
		{"nil stays nil", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalize(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
