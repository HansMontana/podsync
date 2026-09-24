package httpclient

import (
	"net"
	"net/http"
	"net/url"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.1.1", "::1", "fd00::1"} {
		if !isPrivateIP(net.ParseIP(value)) {
			t.Errorf("isPrivateIP(%q) = false", value)
		}
	}
	if isPrivateIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("isPrivateIP() rejected a public address")
	}
}

func TestCheckRedirectRejectsDowngrade(t *testing.T) {
	request := &http.Request{URL: mustURL(t, "http://example.com/feed.xml")}
	if err := checkRedirect(request, []*http.Request{{URL: mustURL(t, "https://example.com/start.xml")}}); err == nil {
		t.Fatal("checkRedirect() accepted an HTTPS downgrade")
	}
}

func TestCheckRedirectDropsConditionalHeadersAcrossOrigins(t *testing.T) {
	request := &http.Request{URL: mustURL(t, "https://other.example/feed.xml"), Header: http.Header{
		"If-None-Match":     []string{"etag"},
		"If-Modified-Since": []string{"date"},
	}}
	if err := checkRedirect(request, []*http.Request{{URL: mustURL(t, "https://example.com/start.xml")}}); err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Modified-Since") != "" {
		t.Fatalf("conditional headers remained: %v", request.Header)
	}
}

func TestNewDisablesEnvironmentProxy(t *testing.T) {
	client := New(0)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport has type %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("New() retained proxy configuration")
	}
}

func TestCheckRedirectRejectsCredentials(t *testing.T) {
	request := &http.Request{URL: mustURL(t, "https://user:password@example.com/feed.xml")}
	if err := checkRedirect(request, []*http.Request{{URL: mustURL(t, "https://example.com/start.xml")}}); err == nil {
		t.Fatal("checkRedirect() accepted redirect credentials")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	value, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
