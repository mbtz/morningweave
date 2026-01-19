package connectors

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestRateLimitDelayRetryAfterSeconds(t *testing.T) {
	now := time.Date(2026, 1, 18, 8, 0, 0, 0, time.UTC)
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"120"}}}

	delay, ok := RateLimitDelay(resp, now)
	if !ok {
		t.Fatalf("expected retry-after to be parsed")
	}
	if delay != 120*time.Second {
		t.Fatalf("expected 120s delay, got %v", delay)
	}
}

func TestRateLimitDelayRetryAfterDate(t *testing.T) {
	now := time.Date(2026, 1, 18, 8, 0, 0, 0, time.UTC)
	retryAt := now.Add(45 * time.Second).UTC().Format(http.TimeFormat)
	resp := &http.Response{Header: http.Header{"Retry-After": []string{retryAt}}}

	delay, ok := RateLimitDelay(resp, now)
	if !ok {
		t.Fatalf("expected retry-after date to be parsed")
	}
	if delay != 45*time.Second {
		t.Fatalf("expected 45s delay, got %v", delay)
	}
}

func TestRateLimitDelayResetHeader(t *testing.T) {
	now := time.Date(2026, 1, 18, 8, 0, 0, 0, time.UTC)
	reset := now.Add(30 * time.Second)
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))

	delay, ok := RateLimitDelay(resp, now)
	if !ok {
		t.Fatalf("expected x-ratelimit-reset to be parsed")
	}
	if delay != 30*time.Second {
		t.Fatalf("expected 30s delay, got %v", delay)
	}
}
