package reddit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"morningweave/internal/connectors"
)

func TestFetchSubredditItems(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newRedditTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithAuthURL(server.URL+"/api/v1/access_token"),
		WithCredentials(testCredentials()),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "subreddits", Identifier: "golang"}},
		Keywords: []string{
			"go",
		},
		Since: now.Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.Title != "Go 1.23 release" {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if item.URL != "https://example.com/go" {
		t.Fatalf("unexpected url: %q", item.URL)
	}
	if item.Source.Platform != "reddit" || item.Source.SourceType != "subreddits" || item.Source.Identifier != "golang" {
		t.Fatalf("unexpected source: %+v", item.Source)
	}
	if item.Engagement.Score != 12 || item.Engagement.Comments != 3 {
		t.Fatalf("unexpected engagement: %+v", item.Engagement)
	}
}

func TestFetchKeywordSearch(t *testing.T) {
	now := time.Date(2026, 1, 18, 12, 0, 0, 0, time.UTC)
	server := newRedditTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithAuthURL(server.URL+"/api/v1/access_token"),
		WithCredentials(testCredentials()),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "keywords", Identifier: "golang"}},
		Keywords: []string{
			"golang",
		},
		Until: now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Source.SourceType != "keywords" {
		t.Fatalf("expected keyword source, got %q", result.Items[0].Source.SourceType)
	}
}

func TestFetchMissingCredentials(t *testing.T) {
	conn := New()
	_, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "subreddits", Identifier: "golang"}},
	})
	if err == nil {
		t.Fatalf("expected auth error")
	}
	if !errors.Is(err, connectors.ErrAuthMissing) {
		t.Fatalf("expected ErrAuthMissing, got %v", err)
	}
}

func newRedditTestServer(t *testing.T, now time.Time) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token", TokenType: "bearer", ExpiresIn: 3600})
	})

	mux.HandleFunc("/r/golang/new", func(w http.ResponseWriter, r *http.Request) {
		payload := listing{Data: listingData{Children: []listingChild{
			{Data: postData{Title: "Go 1.23 release", URL: "https://example.com/go", SelfText: "", Score: 12, NumComments: 3, CreatedUTC: float64(now.Add(-30 * time.Minute).Unix())}},
			{Data: postData{Title: "Rust update", URL: "https://example.com/rust", SelfText: "", Score: 5, NumComments: 1, CreatedUTC: float64(now.Add(-2 * time.Hour).Unix())}},
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		payload := listing{Data: listingData{Children: []listingChild{
			{Data: postData{Title: "Golang tips", URL: "https://example.com/tips", SelfText: "Learn go", Score: 2, NumComments: 0, CreatedUTC: float64(now.Add(-20 * time.Minute).Unix())}},
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	return httptest.NewServer(mux)
}

func testCredentials() Credentials {
	return Credentials{
		ClientID:     "client",
		ClientSecret: "secret",
		UserAgent:    "morningweave-test",
	}
}
