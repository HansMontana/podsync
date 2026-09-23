package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// New returns the HTTP client used for untrusted feed and media URLs.
func New(timeout time.Duration) *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	} else {
		transport = transport.Clone()
	}
	transport.DialContext = dialPublicContext
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
}

func dialPublicContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split remote address: %w", err)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve remote host: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return nil, fmt.Errorf("remote host resolves to a private address")
		}
	}
	var lastErr error
	for _, ip := range ips {
		connection, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, fmt.Errorf("remote host has no addresses")
	}
	return nil, fmt.Errorf("connect to remote host: %w", lastErr)
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

func checkRedirect(next *http.Request, via []*http.Request) error {
	if next.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to non-HTTPS URL")
	}
	if len(via) == 0 {
		return nil
	}
	previous := via[len(via)-1].URL
	if sameOrigin(previous, next.URL) {
		return nil
	}
	next.Header.Del("If-None-Match")
	next.Header.Del("If-Modified-Since")
	return nil
}

func sameOrigin(first, second *url.URL) bool {
	return strings.EqualFold(first.Scheme, second.Scheme) &&
		strings.EqualFold(first.Hostname(), second.Hostname()) &&
		port(first) == port(second)
}

func port(value *url.URL) string {
	if explicit := value.Port(); explicit != "" {
		return explicit
	}
	if strings.EqualFold(value.Scheme, "https") {
		return strconv.Itoa(443)
	}
	return strconv.Itoa(80)
}
