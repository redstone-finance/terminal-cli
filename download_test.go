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
	if err := downloadStream(srv.URL, fullPath); err == nil {
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
	if err := downloadStream(srv.URL, fullPath); err != nil {
		t.Fatalf("downloadStream: %v", err)
	}
	got, err := os.ReadFile(fullPath)
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v; want %q", got, err, "hello")
	}
}

func TestClassify(t *testing.T) {
	defer func(u string) { apiURL = u }(apiURL)
	serve := func(code int) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"x"}`))
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	fetch := func(code int) error {
		apiURL = serve(code)
		_, _, err := fetchDownloadLink("key", "f")
		return err
	}
	aborted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	defer aborted.Close()
	closed := httptest.NewServer(nil)
	closed.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "dir.parquet", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok := serve(http.StatusOK)

	for _, tc := range []struct {
		name string
		err  error
		want outcome
	}{
		{"api 401", fetch(http.StatusUnauthorized), failedAuth},
		{"api 403", fetch(http.StatusForbidden), failedAuth},
		{"api 429", fetch(http.StatusTooManyRequests), failedNetwork},
		{"api 502", fetch(http.StatusBadGateway), failedNetwork},
		{"api 400", fetch(http.StatusBadRequest), failed},
		{"download 503", downloadStream(serve(http.StatusServiceUnavailable), filepath.Join(dir, "a")), failedNetwork},
		{"download 403", downloadStream(serve(http.StatusForbidden), filepath.Join(dir, "b")), failed},
		{"connection refused", downloadStream(closed.URL, filepath.Join(dir, "c")), failedNetwork},
		{"aborted transfer", downloadStream(aborted.URL, filepath.Join(dir, "d")), failedNetwork},
		{"mkdir", downloadStream(ok, filepath.Join(dir, "file", "x.parquet")), failedDisk},
		{"rename", downloadStream(ok, filepath.Join(dir, "dir.parquet")), failedDisk},
	} {
		if got := classify(tc.err); got != tc.want {
			t.Errorf("%s: classify(%v) = %d, want %d", tc.name, tc.err, got, tc.want)
		}
	}
}
