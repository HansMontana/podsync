package feed

import "testing"

func TestNormalizeURLTrimsWhitespace(t *testing.T) {
	got, err := NormalizeURL("  https://example.com/feed.xml  ")
	if err != nil {
		t.Fatal(err)
	}

	want := "https://example.com/feed.xml"

	if got != want {
		t.Fatalf("NormalizeURL() = %q, want %q", got, want)
	}
}

func TestNormalizeURLLowercasesSchemeAndHost(t *testing.T) {
	got, err := NormalizeURL("HTTPS://EXAMPLE.COM/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	want := "https://example.com/feed.xml"

	if got != want {
		t.Fatalf("NormalizeURL() = %q, want %q", got, want)
	}
}

func TestNormalizeURLRemovesDefaultHTTPPort(t *testing.T) {
	got, err := NormalizeURL("http://example.com:80/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	want := "http://example.com/feed.xml"

	if got != want {
		t.Fatalf("NormalizeURL() = %q, want %q", got, want)
	}
}

func TestNormalizeURLRemovesDefaultHTTPSPort(t *testing.T) {
	got, err := NormalizeURL("https://example.com:443/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	want := "https://example.com/feed.xml"

	if got != want {
		t.Fatalf("NormalizeURL() = %q, want %q", got, want)
	}
}

func TestNormalizeURLPreservesQuery(t *testing.T) {
	got, err := NormalizeURL("https://example.com/feed.xml?format=rss")
	if err != nil {
		t.Fatal(err)
	}

	want := "https://example.com/feed.xml?format=rss"

	if got != want {
		t.Fatalf("NormalizeURL() = %q, want %q", got, want)
	}
}

func TestNormalizeURLRejectsMalformedURL(t *testing.T) {
	_, err := NormalizeURL("://not-a-url")
	if err == nil {
		t.Fatal("NormalizeURL() expected an error for malformed URL")
	}
}

func TestNormalizeURLRejectsMissingScheme(t *testing.T) {
	_, err := NormalizeURL("example.com/feed.xml")
	if err == nil {
		t.Fatal("NormalizeURL() expected an error for URL without scheme")
	}
}

func TestNormalizeURLRejectsCredentials(t *testing.T) {
	if _, err := NormalizeURL("https://user:secret@example.com/feed.xml"); err == nil {
		t.Fatal("NormalizeURL accepted credentials")
	}
}

func TestFeedSameIdentityUsesNormalizedURL(t *testing.T) {
	first := Feed{
		URL: " HTTPS://EXAMPLE.COM:443/feed.xml ",
	}

	second := Feed{
		URL: "https://example.com/feed.xml",
	}

	if !first.SameIdentity(second) {
		t.Fatal("feeds with equivalent normalized URLs should have the same identity")
	}
}

func TestFeedDifferentURLsHaveDifferentIdentity(t *testing.T) {
	first := Feed{
		URL: "https://example.com/feed-a.xml",
	}

	second := Feed{
		URL: "https://example.com/feed-b.xml",
	}

	if first.SameIdentity(second) {
		t.Fatal("feeds with different URLs should not have the same identity")
	}
}

func TestFeedNameDoesNotAffectIdentity(t *testing.T) {
	first := Feed{
		Name: "ATP",
		URL:  "https://example.com/feed.xml",
	}

	second := Feed{
		Name: "Accidental Tech Podcast",
		URL:  "https://example.com/feed.xml",
	}

	if !first.SameIdentity(second) {
		t.Fatal("changing the feed name should not change feed identity")
	}
}
