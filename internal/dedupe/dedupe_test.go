package dedupe

import (
	"testing"

	"morningweave/internal/connectors"
)

func TestTitleSimilarity(t *testing.T) {
	if TitleSimilarity("Go 1.22 released", "go 1 22 released") < 0.8 {
		t.Fatalf("expected similar titles to score higher")
	}
	if TitleSimilarity("AI model launch", "gardening tips") >= 0.5 {
		t.Fatalf("expected dissimilar titles to score lower")
	}
}

func TestDedupeByCanonicalURL(t *testing.T) {
	items := []connectors.Item{
		{
			Title: "Post A",
			URL:   "https://example.com/post?utm_source=x",
			Engagement: connectors.Engagement{
				Likes: 2,
			},
			Source: connectors.SourceRef{Platform: "reddit", SourceType: "subreddit", Identifier: "golang"},
		},
		{
			Title: "Post B",
			URL:   "https://example.com/post",
			Engagement: connectors.Engagement{
				Likes: 10,
			},
			Source: connectors.SourceRef{Platform: "hn", SourceType: "list", Identifier: "top"},
		},
	}

	merged := Dedupe(items)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(merged))
	}
	if merged[0].Item.Title != "Post B" {
		t.Fatalf("expected highest engagement item to win, got %q", merged[0].Item.Title)
	}
	if len(merged[0].Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(merged[0].Sources))
	}
	if merged[0].CanonicalURL != "https://example.com/post" {
		t.Fatalf("expected canonical URL, got %q", merged[0].CanonicalURL)
	}
}

func TestDedupeByTitleSimilarity(t *testing.T) {
	items := []connectors.Item{
		{
			Title: "OpenAI releases model",
			URL:   "https://news.example.com/a",
			Engagement: connectors.Engagement{
				Score: 5,
			},
			Source: connectors.SourceRef{Platform: "x", SourceType: "users", Identifier: "openai"},
		},
		{
			Title: "OpenAI releases model!",
			URL:   "https://blog.example.com/b",
			Engagement: connectors.Engagement{
				Score: 8,
			},
			Source: connectors.SourceRef{Platform: "reddit", SourceType: "subreddit", Identifier: "ai"},
		},
	}

	merged := Dedupe(items)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(merged))
	}
	if len(merged[0].Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(merged[0].Sources))
	}
	if merged[0].Item.Title != "OpenAI releases model!" {
		t.Fatalf("expected highest engagement title to win, got %q", merged[0].Item.Title)
	}
}
