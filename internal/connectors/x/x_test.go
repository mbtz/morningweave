package x

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"morningweave/internal/connectors"
)

func TestFetchKeywordSearch(t *testing.T) {
	now := time.Date(2026, 1, 18, 9, 0, 0, 0, time.UTC)
	server := newXTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithCredentials(Credentials{BearerToken: "token"}),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "keywords", Identifier: "golang"}},
		Keywords: []string{
			"go",
		},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.URL != "https://x.com/gopher/status/123" {
		t.Fatalf("unexpected url: %q", item.URL)
	}
	if item.Source.SourceType != "keywords" {
		t.Fatalf("unexpected source type: %q", item.Source.SourceType)
	}
	if item.Engagement.Likes != 5 || item.Engagement.Reposts != 3 {
		t.Fatalf("unexpected engagement: %+v", item.Engagement)
	}
}

func TestFetchUserTimeline(t *testing.T) {
	now := time.Date(2026, 1, 18, 10, 0, 0, 0, time.UTC)
	server := newXTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithCredentials(Credentials{BearerToken: "token"}),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "users", Identifier: "openai"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].URL != "https://x.com/openai/status/555" {
		t.Fatalf("unexpected url: %q", result.Items[0].URL)
	}
}

func TestFetchListRateLimit(t *testing.T) {
	now := time.Date(2026, 1, 18, 11, 0, 0, 0, time.UTC)
	server := newXTestServer(t, now)
	defer server.Close()

	conn := New(
		WithBaseURL(server.URL),
		WithCredentials(Credentials{BearerToken: "token"}),
		WithNow(func() time.Time { return now }),
	)

	result, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "lists", Identifier: "99"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if result.RateLimit == nil {
		t.Fatalf("expected rate limit info")
	}
	if result.RateLimit.Limit != 300 || result.RateLimit.Remaining != 295 {
		t.Fatalf("unexpected rate limit: %+v", result.RateLimit)
	}
}

func TestFetchMissingCredentials(t *testing.T) {
	conn := New()
	_, err := conn.Fetch(context.Background(), connectors.FetchRequest{
		Sources: []connectors.Source{{SourceType: "keywords", Identifier: "golang"}},
	})
	if err == nil {
		t.Fatalf("expected auth error")
	}
	if !errors.Is(err, connectors.ErrAuthMissing) {
		t.Fatalf("expected ErrAuthMissing, got %v", err)
	}
}

func newXTestServer(t *testing.T, now time.Time) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/2/tweets/search/recent", func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
			t.Fatalf("expected bearer token, got %q", auth)
		}
		if query := r.URL.Query().Get("query"); query != "golang" {
			t.Fatalf("unexpected query: %q", query)
		}
		payload := apiResponse{
			Data: []tweet{
				{
					ID:        "123",
					Text:      "Go 1.23 release announcement",
					CreatedAt: now.Add(-30 * time.Minute),
					AuthorID:  "u1",
					Lang:      "en",
					PublicMetrics: publicMetrics{
						LikeCount:    5,
						RetweetCount: 2,
						QuoteCount:   1,
						ReplyCount:   0,
					},
				},
				{
					ID:        "124",
					Text:      "Completely unrelated",
					CreatedAt: now.Add(-10 * time.Minute),
					AuthorID:  "u2",
					Lang:      "en",
					PublicMetrics: publicMetrics{
						LikeCount: 1,
					},
				},
			},
			Includes: includes{Users: []user{{ID: "u1", Username: "gopher"}, {ID: "u2", Username: "other"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/2/users/by/username/openai", func(w http.ResponseWriter, r *http.Request) {
		payload := userLookupResponse{
			Data: user{ID: "42", Username: "openai"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/2/users/42/tweets", func(w http.ResponseWriter, r *http.Request) {
		payload := apiResponse{
			Data: []tweet{
				{
					ID:        "555",
					Text:      "OpenAI update",
					CreatedAt: now.Add(-45 * time.Minute),
					AuthorID:  "42",
					Lang:      "en",
				},
			},
			Includes: includes{Users: []user{{ID: "42", Username: "openai"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/2/lists/99/tweets", func(w http.ResponseWriter, r *http.Request) {
		reset := now.Add(5 * time.Minute)
		w.Header().Set("x-rate-limit-limit", "300")
		w.Header().Set("x-rate-limit-remaining", "295")
		w.Header().Set("x-rate-limit-reset", strconv.FormatInt(reset.Unix(), 10))
		payload := apiResponse{
			Data: []tweet{
				{
					ID:        "888",
					Text:      "List tweet",
					CreatedAt: now.Add(-15 * time.Minute),
					AuthorID:  "u9",
					Lang:      "en",
				},
			},
			Includes: includes{Users: []user{{ID: "u9", Username: "listuser"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	return httptest.NewServer(mux)
}
