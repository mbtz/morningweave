package instagram

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

func TestFetchAccountByUsername(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newInstagramTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithCredentials(Credentials{AccessToken: "token"}),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "accounts", Identifier: "openai"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.URL != "https://instagram.com/p/abc" {
		t.Fatalf("unexpected url: %q", item.URL)
	}
	if item.Source.SourceType != "accounts" || item.Source.Identifier != "openai" {
		t.Fatalf("unexpected source: %+v", item.Source)
	}
	if item.Engagement.Likes != 12 || item.Engagement.Comments != 3 {
		t.Fatalf("unexpected engagement: %+v", item.Engagement)
	}
}

func TestFetchHashtag(t *testing.T) {
	now := time.Date(2026, 1, 18, 10, 0, 0, 0, time.UTC)
	server := newInstagramTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithCredentials(Credentials{AccessToken: "token"}),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "hashtags", Identifier: "#golang"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.Source.SourceType != "hashtags" || item.Source.Identifier != "golang" {
		t.Fatalf("unexpected source: %+v", item.Source)
	}
	if item.URL != "https://instagram.com/p/hashtag" {
		t.Fatalf("unexpected url: %q", item.URL)
	}
}

func TestFetchMissingCredentials(t *testing.T) {
	conn := New()
	_, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "accounts", Identifier: "openai"}},
	})
	if err == nil {
		t.Fatalf("expected auth error")
	}
	if err != connectors.ErrAuthMissing {
		t.Fatalf("expected ErrAuthMissing, got %v", err)
	}
}

func newInstagramTestServer(t *testing.T, now time.Time) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		if token := r.URL.Query().Get("access_token"); token != "token" {
			t.Fatalf("unexpected access_token: %q", token)
		}
		payload := map[string]any{"id": "123", "username": "biz"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/123", func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query().Get("fields")
		if !strings.Contains(fields, "business_discovery.username(openai)") {
			t.Fatalf("unexpected fields: %q", fields)
		}
		payload := map[string]any{
			"business_discovery": map[string]any{
				"media": map[string]any{
					"data": []map[string]any{
						{
							"id":             "m1",
							"caption":        "Go 1.23 released!",
							"media_type":     "IMAGE",
							"permalink":      "https://instagram.com/p/abc",
							"timestamp":      now.Format(time.RFC3339),
							"like_count":     12,
							"comments_count": 3,
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/ig_hashtag_search", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "golang" {
			t.Fatalf("unexpected hashtag query: %q", got)
		}
		if got := r.URL.Query().Get("user_id"); got != "123" {
			t.Fatalf("unexpected user_id: %q", got)
		}
		payload := map[string]any{
			"data": []map[string]any{{"id": "55", "name": "golang"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/55/recent_media", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("user_id"); got != "123" {
			t.Fatalf("unexpected user_id: %q", got)
		}
		payload := map[string]any{
			"data": []map[string]any{
				{
					"id":               "m2",
					"caption":          "Gophers rock",
					"media_type":       "VIDEO",
					"permalink":        "https://instagram.com/p/hashtag",
					"timestamp":        now.Add(-1 * time.Hour).Format(time.RFC3339),
					"like_count":       9,
					"comments_count":   1,
					"video_view_count": 100,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	return httptest.NewServer(mux)
}
