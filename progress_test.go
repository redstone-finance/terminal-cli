package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/pterm/pterm"
)

// Throttling must not lose bytes: the final short read flushes the remainder.
func TestProgressReaderCountsEveryByte(t *testing.T) {
	const size = 1 << 20

	var out bytes.Buffer
	bar, err := pterm.DefaultProgressbar.WithWriter(&out).WithTotal(size + 1).Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = bar.Stop() }()

	pr := &ProgressReader{Reader: strings.NewReader(strings.Repeat("x", size)), Bar: bar}
	if _, err := io.Copy(io.Discard, pr); err != nil {
		t.Fatal(err)
	}

	if bar.Current != size {
		t.Errorf("bar.Current = %d, want %d", bar.Current, size)
	}
}
