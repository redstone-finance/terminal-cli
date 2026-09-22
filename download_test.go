package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// An aborted transfer must leave no file behind, or the "Skipped (Exists)"
// check in processJob would treat the truncated file as a complete download.
func TestDownloadStreamLeavesNoFileOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	defer srv.Close()

	fullPath := filepath.Join(t.TempDir(), "x.parquet")
	if err := downloadStream(srv.URL, fullPath, nil); err == nil {
		t.Fatal("expected an error from the aborted transfer")
	}
	for _, p := range []string{fullPath, fullPath + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should not exist after a failed download", p)
		}
	}
}

func TestDownloadStreamWritesFileOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	fullPath := filepath.Join(t.TempDir(), "nested", "x.parquet")
	if err := downloadStream(srv.URL, fullPath, nil); err != nil {
		t.Fatalf("downloadStream: %v", err)
	}
	got, err := os.ReadFile(fullPath)
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v; want %q", got, err, "hello")
	}
}
