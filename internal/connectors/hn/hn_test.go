package hn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"morningweave/internal/connectors"
)

func TestFetchListItems(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newHNTestServer(t, map[string]any{
		"/v0/topstories.json": []int{101},
		"/v0/item/101.json": hnItem{
			ID:          101,
			Type:        "story",
			Title:       "Hello World",
			URL:         "https://example.com/hello",
			Text:        "<p>Example summary</p>",
			Time:        now.Unix(),
			Score:       42,
			Descendants: 5,
		},
	})
	defer server.Close()

	conn := New(WithBaseURL(server.URL + "/v0"))
	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{
			{SourceType: "lists", Identifier: "top"},
		},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.Title != "Hello World" {
		t.Fatalf("title mismatch: %q", item.Title)
	}
	if item.URL != "https://example.com/hello" {
		t.Fatalf("url mismatch: %q", item.URL)
	}
	if item.Text != "Example summary" {
		t.Fatalf("text mismatch: %q", item.Text)
	}
	if item.Engagement.Score != 42 || item.Engagement.Comments != 5 {
		t.Fatalf("engagement mismatch: %+v", item.Engagement)
	}
	if item.Source.Platform != "hn" || item.Source.SourceType != "lists" || item.Source.Identifier != "top" {
		t.Fatalf("source mismatch: %+v", item.Source)
	}
}

func TestFetchKeywordFiltering(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newHNTestServer(t, map[string]any{
		"/v0/topstories.json": []int{201, 202},
		"/v0/item/201.json": hnItem{
			ID:    201,
			Type:  "story",
			Title: "Go 1.23 released",
			Text:  "Details about the release",
			Time:  now.Unix(),
		},
		"/v0/item/202.json": hnItem{
			ID:    202,
			Type:  "story",
			Title: "Rust update",
			Text:  "Borrow checker news",
			Time:  now.Unix(),
		},
	})
	defer server.Close()

	conn := New(WithBaseURL(server.URL + "/v0"))
	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{
			{SourceType: "lists", Identifier: "top"},
		},
		Keywords: []string{"Go"},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if !strings.Contains(strings.ToLower(result.Items[0].Title), "go") {
		t.Fatalf("unexpected item: %q", result.Items[0].Title)
	}
}

func TestFetchKeywordSources(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newHNTestServer(t, map[string]any{
		"/v0/beststories.json": []int{301, 302},
		"/v0/item/301.json": hnItem{
			ID:    301,
			Type:  "story",
			Title: "AI release",
			Text:  "LLM news",
			Time:  now.Unix(),
		},
		"/v0/item/302.json": hnItem{
			ID:    302,
			Type:  "story",
			Title: "Cooking tips",
			Text:  "Food blog",
			Time:  now.Unix(),
		},
	})
	defer server.Close()

	conn := New(WithBaseURL(server.URL + "/v0"))
	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{
			{SourceType: "list", Identifier: "best"},
			{SourceType: "keywords", Identifier: "AI, ml"},
		},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if !strings.Contains(strings.ToLower(result.Items[0].Title), "ai") {
		t.Fatalf("unexpected item: %q", result.Items[0].Title)
	}
}

func TestFetchSinceUntilAndFallbackURL(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newHNTestServer(t, map[string]any{
		"/v0/newstories.json": []int{401, 402},
		"/v0/item/401.json": hnItem{
			ID:    401,
			Type:  "story",
			Title: "Old story",
			Text:  "Old text",
			Time:  now.Add(-2 * time.Hour).Unix(),
		},
		"/v0/item/402.json": hnItem{
			ID:    402,
			Type:  "story",
			Title: "Recent story",
			Text:  "Fresh text",
			Time:  now.Add(-30 * time.Minute).Unix(),
		},
	})
	defer server.Close()

	conn := New(WithBaseURL(server.URL + "/v0"))
	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{
			{SourceType: "lists", Identifier: "newstories"},
		},
		Since: now.Add(-1 * time.Hour),
		Until: now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].URL != "https://news.ycombinator.com/item?id=402" {
		t.Fatalf("expected fallback url, got %q", result.Items[0].URL)
	}
}

func TestFetchMissingListSources(t *testing.T) {
	conn := New()
	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Keywords: []string{"golang"},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected no items, got %d", len(result.Items))
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warning for missing list sources")
	}
}

func newHNTestServer(t *testing.T, responses map[string]any) *httptest.Server {
	t.Helper()
	handler := http.NewServeMux()
	for path, payload := range responses {
		path := path
		payload := payload
		handler.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(payload); err != nil {
				t.Fatalf("encode payload: %v", err)
			}
		})
	}
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return httptest.NewServer(handler)
}
