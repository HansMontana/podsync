package feed

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRSSUsesConditionalHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-None-Match"); got != `"feed-1"` {
			t.Errorf("If-None-Match = %q, want %q", got, `"feed-1"`)
		}
		if got := r.Header.Get("If-Modified-Since"); got != "Wed, 02 Sep 2026 14:39:34 +0200" {
			t.Errorf("If-Modified-Since = %q", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	result, err := FetchRSS(context.Background(), server.Client(), Feed{
		URL:          server.URL,
		ETag:         `"feed-1"`,
		LastModified: "Wed, 02 Sep 2026 14:39:34 +0200",
	})
	if err != nil {
		t.Fatalf("FetchRSS() returned error: %v", err)
	}
	if !result.NotModified {
		t.Fatal("FetchRSS() reported a modified feed for a 304 response")
	}
}

func TestFetchRSSReturnsBodyAndCacheHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"feed-2"`)
		w.Header().Set("Last-Modified", "Wed, 02 Sep 2026 14:39:34 +0200")
		_, _ = w.Write([]byte("<rss/>"))
	}))
	defer server.Close()

	result, err := FetchRSS(context.Background(), server.Client(), Feed{URL: server.URL})
	if err != nil {
		t.Fatalf("FetchRSS() returned error: %v", err)
	}
	if string(result.Body) != "<rss/>" {
		t.Fatalf("got body %q", result.Body)
	}
	if result.ETag != `"feed-2"` || result.LastModified != "Wed, 02 Sep 2026 14:39:34 +0200" {
		t.Fatalf("got cache metadata %+v", result)
	}
}

func TestFetchRSSAcceptsLargerRealWorldFeeds(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 11<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	result, err := FetchRSS(context.Background(), server.Client(), Feed{URL: server.URL})
	if err != nil {
		t.Fatalf("FetchRSS() rejected a feed larger than the old limit: %v", err)
	}
	if len(result.Body) != len(body) {
		t.Fatalf("got body length %d, want %d", len(result.Body), len(body))
	}
}

func TestFetchRSSRejectsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	if _, err := FetchRSS(context.Background(), server.Client(), Feed{URL: server.URL}); err == nil {
		t.Fatal("FetchRSS() accepted a forbidden response")
	}
}
