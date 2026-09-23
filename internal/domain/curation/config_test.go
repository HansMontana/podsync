package curation

import (
	"strings"
	"testing"
)

func TestValidateRejectsPlaylistFilenameCollision(t *testing.T) {
	cfg := Config{
		Sources: []SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds: []LogicalFeed{
			{ID: "morning", Title: "Daily News", Source: "news", Order: NewestFirst},
			{ID: "evening", Title: "Daily/News", Source: "news", Order: NewestFirst},
		},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "playlist filename") {
		t.Fatalf("Validate() error = %v", err)
	}
}
