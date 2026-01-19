package email

import (
	"strings"
	"testing"
	"time"

	"morningweave/internal/connectors"
	"morningweave/internal/dedupe"
)

func TestRenderDigestWordCap(t *testing.T) {
	items := []dedupe.MergedItem{
		makeMerged("First item", "one two three four five six", "https://example.com/1", "reddit"),
		makeMerged("Second item", "seven eight nine ten eleven", "https://example.com/2", "x"),
	}

	result, err := RenderDigest(items, RenderOptions{Title: "Test", WordCap: 7, MaxItems: 10, GeneratedAt: time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Items != 1 {
		t.Fatalf("expected 1 item, got %d", result.Items)
	}
	if result.Words > 7 {
		t.Fatalf("expected words <= 7, got %d", result.Words)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated true")
	}
	if !strings.Contains(result.HTML, "one two three four five...") {
		t.Fatalf("expected truncated excerpt in HTML")
	}
}

func TestRenderDigestMaxItems(t *testing.T) {
	items := []dedupe.MergedItem{
		makeMerged("First item", "one two three", "https://example.com/1", "reddit"),
		makeMerged("Second item", "four five six", "https://example.com/2", "x"),
	}

	result, err := RenderDigest(items, RenderOptions{Title: "Test", WordCap: 100, MaxItems: 1})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Items != 1 {
		t.Fatalf("expected 1 item, got %d", result.Items)
	}
	if strings.Contains(result.HTML, "Second item") {
		t.Fatalf("did not expect second item in HTML")
	}
}

func makeMerged(title, text, url, platform string) dedupe.MergedItem {
	item := connectors.Item{
		Title: title,
		Text:  text,
		URL:   url,
		Source: connectors.SourceRef{
			Platform:   platform,
			SourceType: "subreddit",
			Identifier: "golang",
		},
	}
	return dedupe.MergedItem{
		Item:         item,
		CanonicalURL: url,
		Sources: []dedupe.SourceLink{
			{
				Source: item.Source,
				URL:    url,
			},
		},
	}
}
