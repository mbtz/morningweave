package connectors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRetries = 3
	defaultBaseDelay  = 500 * time.Millisecond
	defaultMaxDelay   = 5 * time.Second
	defaultTimeout    = 15 * time.Second
)

// RetryConfig controls retry behavior for connector HTTP calls.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Timeout    time.Duration
}

// DefaultRetryConfig provides sensible defaults for connector HTTP calls.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: defaultMaxRetries,
		BaseDelay:  defaultBaseDelay,
		MaxDelay:   defaultMaxDelay,
		Timeout:    defaultTimeout,
	}
}

// HTTPClient wraps http.Client with retry/backoff and rate-limit handling.
type HTTPClient struct {
	client *http.Client
	cfg    RetryConfig
	jit    *rand.Rand
	now    func() time.Time
	sleep  func(time.Duration)
}

// NewHTTPClient creates a new retry-capable client.
func NewHTTPClient(cfg RetryConfig) *HTTPClient {
	applied := applyRetryDefaults(cfg)
	return &HTTPClient{
		client: &http.Client{Timeout: applied.Timeout},
		cfg:    applied,
		jit:    rand.New(rand.NewSource(time.Now().UnixNano())),
		now:    time.Now,
		sleep:  time.Sleep,
	}
}

// Do executes the request with retry/backoff for transient and rate-limited responses.
//
// Requests with bodies must provide GetBody for retries to be possible.
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if ctx == nil {
		return nil, errors.New("context is nil")
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		attemptReq, err := c.buildRequest(ctx, req, attempt)
		if err != nil {
			return nil, err
		}

		resp, err := c.client.Do(attemptReq)
		if err == nil && !shouldRetryResponse(resp) {
			return resp, nil
		}

		delay, retry := c.retryDelay(attempt, resp, err)
		if !retry {
			if err != nil {
				return nil, err
			}
			if resp != nil {
				return resp, nil
			}
			return nil, errors.New("request failed without response")
		}

		if err == nil && resp != nil {
			_ = drainAndClose(resp.Body)
		}

		lastErr = err
		if delay > 0 {
			if err := sleepContext(ctx, delay, c.sleep); err != nil {
				return nil, err
			}
		}
	}

	if lastErr == nil {
		lastErr = errors.New("request failed after retries")
	}
	return nil, lastErr
}

func (c *HTTPClient) buildRequest(ctx context.Context, original *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 {
		return original.WithContext(ctx), nil
	}
	clone, err := cloneRequest(original, ctx)
	if err != nil {
		return nil, err
	}
	return clone, nil
}

func cloneRequest(req *http.Request, ctx context.Context) (*http.Request, error) {
	clone := req.Clone(ctx)
	if req.Body == nil {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("request body for %s %s is not rewindable; set GetBody to enable retries", req.Method, req.URL.String())
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}

func shouldRetryResponse(resp *http.Response) bool {
	if resp == nil {
		return true
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		return true
	}
	return false
}

func (c *HTTPClient) retryDelay(attempt int, resp *http.Response, err error) (time.Duration, bool) {
	if attempt >= c.cfg.MaxRetries {
		return 0, false
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, false
		}
		return c.backoffDelay(attempt), true
	}
	if resp == nil {
		return c.backoffDelay(attempt), true
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		if delay, ok := RateLimitDelay(resp, c.now()); ok {
			return delay, true
		}
		return c.backoffDelay(attempt), true
	}
	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		return c.backoffDelay(attempt), true
	}
	return 0, false
}

func (c *HTTPClient) backoffDelay(attempt int) time.Duration {
	base := c.cfg.BaseDelay
	if base <= 0 {
		base = defaultBaseDelay
	}
	maxDelay := c.cfg.MaxDelay
	if maxDelay <= 0 {
		maxDelay = defaultMaxDelay
	}

	delay := base << attempt
	if delay > maxDelay {
		delay = maxDelay
	}
	if delay <= 0 {
		return 0
	}

	jitter := time.Duration(c.jit.Int63n(int64(delay/2) + 1))
	return delay/2 + jitter
}

// RateLimitDelay returns a delay derived from rate-limit headers.
func RateLimitDelay(resp *http.Response, now time.Time) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}

	if retryAfter := stringsTrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			if seconds <= 0 {
				return 0, false
			}
			return time.Duration(seconds) * time.Second, true
		}
		if parsed, err := http.ParseTime(retryAfter); err == nil {
			if parsed.After(now) {
				return parsed.Sub(now), true
			}
		}
	}

	for _, header := range []string{"X-RateLimit-Reset", "X-Rate-Limit-Reset"} {
		if value := stringsTrimSpace(resp.Header.Get(header)); value != "" {
			if epoch, err := strconv.ParseInt(value, 10, 64); err == nil {
				reset := time.Unix(epoch, 0)
				if reset.After(now) {
					return reset.Sub(now), true
				}
			}
		}
	}

	return 0, false
}

func applyRetryDefaults(cfg RetryConfig) RetryConfig {
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = defaultBaseDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = defaultMaxDelay
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return cfg
}

func drainAndClose(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	_, _ = io.Copy(io.Discard, body)
	return body.Close()
}

func sleepContext(ctx context.Context, delay time.Duration, sleeper func(time.Duration)) error {
	if delay <= 0 {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func stringsTrimSpace(value string) string {
	if value == "" {
		return ""
	}
	return strings.TrimSpace(value)
}
