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

func TestParseCredentialsRawToken(t *testing.T) {
	creds, err := ParseCredentials("raw-token-123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "raw-token-123" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsBearerPrefix(t *testing.T) {
	creds, err := ParseCredentials("Bearer token-321")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-321" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsAliasKey(t *testing.T) {
	creds, err := ParseCredentials("x-api-key: token-456")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-456" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsAliasKeyTypo(t *testing.T) {
	creds, err := ParseCredentials("x-ap-key: token-457")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-457" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsJSONAlias(t *testing.T) {
	creds, err := ParseCredentials(`{"x-api-key":"token-789"}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-789" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsOPItemFields(t *testing.T) {
	payload := `{"id":"item-123","fields":[{"label":"x-api-key","value":"token-op-1"}]}`
	creds, err := ParseCredentials(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-op-1" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsOPNotesPlain(t *testing.T) {
	payload := `{"id":"item-456","notesPlain":"x-api-key: token-op-2"}`
	creds, err := ParseCredentials(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-op-2" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
	}
}

func TestParseCredentialsOPNestedFieldValue(t *testing.T) {
	payload := `{"id":"item-789","fields":[{"label":"x-api-key","value":{"value":"token-op-3"}}]}`
	creds, err := ParseCredentials(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.BearerToken != "token-op-3" {
		t.Fatalf("unexpected token: %q", creds.BearerToken)
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
