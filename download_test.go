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
	server := func(h http.HandlerFunc) string {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return srv.URL
	}
	serve := func(code int, body string) string {
		return server(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		})
	}
	fetch := func(url string) error {
		apiURL = url
		_, _, err := fetchDownloadLink("key", "f")
		return err
	}
	status := func(code int) error { return fetch(serve(code, `{"message":"x"}`)) }
	aborted := server(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte(`{"download_url":`))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	})
	loop := server(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})
	untrusted := httptest.NewTLSServer(http.NotFoundHandler())
	defer untrusted.Close()
	closed := httptest.NewServer(nil)
	closed.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "dir.parquet", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok := serve(http.StatusOK, "")
	download := func(url string) error {
		return downloadStream(url, filepath.Join(t.TempDir(), "x.parquet"))
	}

	for _, tc := range []struct {
		name string
		err  error
		want outcome
	}{
		{"api 401", status(http.StatusUnauthorized), failedAuth},
		{"api 403", status(http.StatusForbidden), failed},
		{"api 429", status(http.StatusTooManyRequests), failedNetwork},
		{"api 502", status(http.StatusBadGateway), failedNetwork},
		{"api 400", status(http.StatusBadRequest), failed},
		{"api no download_url", fetch(serve(http.StatusOK, `{}`)), failed},
		{"api cut-off json", fetch(aborted), failedNetwork},
		{"download 503", download(serve(http.StatusServiceUnavailable, "")), failedNetwork},
		{"download 429", download(serve(http.StatusTooManyRequests, "")), failedNetwork},
		{"download 408", download(serve(http.StatusRequestTimeout, "")), failedNetwork},
		{"download 403", download(serve(http.StatusForbidden, "")), failed},
		{"connection refused", download(closed.URL), failedNetwork},
		{"aborted transfer", download(aborted), failedNetwork},
		{"untrusted certificate", download(untrusted.URL), failed},
		{"redirect loop", download(loop), failed},
		{"empty url", download(""), failed},
		{"mkdir", downloadStream(ok, filepath.Join(dir, "file", "x.parquet")), failedDisk},
		{"rename", downloadStream(ok, filepath.Join(dir, "dir.parquet")), failedDisk},
	} {
		if got := classify(tc.err); got != tc.want {
			t.Errorf("%s: classify(%v) = %d, want %d", tc.name, tc.err, got, tc.want)
		}
	}
}
