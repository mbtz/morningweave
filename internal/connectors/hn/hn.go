package hn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"morningweave/internal/connectors"
)

const (
	defaultBaseURL    = "https://hacker-news.firebaseio.com/v0"
	defaultMaxItems   = 50
	defaultMaxListIDs = 500
)

var listEndpoints = map[string]string{
	"top":  "topstories",
	"best": "beststories",
	"new":  "newstories",
}

// Connector implements the Hacker News API integration.
type Connector struct {
	client     *connectors.HTTPClient
	baseURL    string
	now        func() time.Time
	maxListIDs int
}

// Option customizes a Connector.
type Option func(*Connector)

// WithHTTPClient overrides the HTTP client used for API calls.
func WithHTTPClient(client *connectors.HTTPClient) Option {
	return func(c *Connector) {
		if client != nil {
			c.client = client
		}
	}
}

// WithBaseURL overrides the API base URL (primarily for tests).
func WithBaseURL(baseURL string) Option {
	return func(c *Connector) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithNow overrides the clock used for timestamps (primarily for tests).
func WithNow(now func() time.Time) Option {
	return func(c *Connector) {
		if now != nil {
			c.now = now
		}
	}
}

// WithMaxListIDs caps how many IDs from a list will be considered.
func WithMaxListIDs(limit int) Option {
	return func(c *Connector) {
		if limit > 0 {
			c.maxListIDs = limit
		}
	}
}

// New constructs a Hacker News connector with defaults applied.
func New(opts ...Option) *Connector {
	c := &Connector{
		client:     connectors.NewHTTPClient(connectors.DefaultRetryConfig()),
		baseURL:    defaultBaseURL,
		now:        time.Now,
		maxListIDs: defaultMaxListIDs,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Platform returns the platform key for Hacker News.
func (c *Connector) Platform() string {
	return "hn"
}

// Requirements describes Hacker News auth requirements (none).
func (c *Connector) Requirements() connectors.Requirements {
	return connectors.Requirements{
		Auth: connectors.AuthRequirements{Required: false},
	}
}

// Status reports connector status (auth-free, no rate limit metadata).
func (c *Connector) Status(ctx context.Context) (connectors.Status, error) {
	_ = ctx
	return connectors.Status{
		Auth: connectors.AuthStatus{Configured: true},
	}, nil
}

// Fetch retrieves items from the configured lists and applies keyword/time filtering.
func (c *Connector) Fetch(ctx context.Context, req connectors.FetchRequest) (connectors.FetchResult, error) {
	result := connectors.FetchResult{}

	listSources, warnings := normalizeListSources(req.Sources)
	result.Warnings = append(result.Warnings, warnings...)

	keywords := normalizeKeywords(req.Keywords, req.Sources)
	if len(listSources) == 0 {
		if len(keywords) > 0 {
			result.Warnings = append(result.Warnings, "hn: no list sources configured; keywords ignored")
		} else {
			result.Warnings = append(result.Warnings, "hn: no list sources configured")
		}
		return result, nil
	}

	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}

	since := req.Since
	until := req.Until
	var listErrors []error
	successfulLists := 0

	for _, source := range listSources {
		ids, err := c.fetchListIDs(ctx, source.Name)
		if err != nil {
			listErrors = append(listErrors, err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("hn: failed to fetch %s list: %v", source.Name, err))
			continue
		}
		successfulLists++

		if c.maxListIDs > 0 && len(ids) > c.maxListIDs {
			ids = ids[:c.maxListIDs]
		}

		for _, id := range ids {
			if maxItems > 0 && len(result.Items) >= maxItems {
				break
			}
			item, err := c.fetchItem(ctx, id)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("hn: failed to fetch item %d: %v", id, err))
				continue
			}
			if item.Deleted || item.Dead {
				continue
			}
			if item.Type != "story" && item.Type != "job" && item.Type != "poll" {
				continue
			}

			timestamp := time.Unix(item.Time, 0)
			if !since.IsZero() && timestamp.Before(since) {
				continue
			}
			if !until.IsZero() && timestamp.After(until) {
				continue
			}

			title := strings.TrimSpace(item.Title)
			if title == "" {
				continue
			}
			text := stripHTML(strings.TrimSpace(item.Text))
			searchText := strings.ToLower(strings.TrimSpace(title + " " + text))
			if len(keywords) > 0 && !containsAny(searchText, keywords) {
				continue
			}

			url := strings.TrimSpace(item.URL)
			if url == "" {
				url = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", item.ID)
			}

			result.Items = append(result.Items, connectors.Item{
				Title: title,
				URL:   url,
				Text:  text,
				Engagement: connectors.Engagement{
					Score:    item.Score,
					Comments: item.Descendants,
				},
				Timestamp: timestamp,
				Source: connectors.SourceRef{
					Platform:   "hn",
					SourceType: source.SourceType,
					Identifier: source.Name,
				},
			})
		}

		if maxItems > 0 && len(result.Items) >= maxItems {
			break
		}
	}

	if successfulLists == 0 && len(listErrors) > 0 {
		return result, listErrors[0]
	}

	return result, nil
}

type listSource struct {
	Name       string
	SourceType string
}

func normalizeListSources(sources []connectors.Source) ([]listSource, []string) {
	if len(sources) == 0 {
		return nil, nil
	}

	listTypes := map[string]struct{}{"list": {}, "lists": {}}
	deduped := map[string]listSource{}
	var warnings []string

	for _, source := range sources {
		sourceType := strings.ToLower(strings.TrimSpace(source.SourceType))
		identifier := strings.TrimSpace(source.Identifier)
		if sourceType == "" {
			warnings = append(warnings, "hn: source type missing; expected lists")
			continue
		}
		if _, ok := listTypes[sourceType]; !ok {
			continue
		}
		if identifier == "" {
			warnings = append(warnings, "hn: list identifier missing")
			continue
		}
		normalized, ok := normalizeListIdentifier(identifier)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("hn: unsupported list %q", identifier))
			continue
		}
		deduped[normalized] = listSource{Name: normalized, SourceType: sourceType}
	}

	lists := make([]listSource, 0, len(deduped))
	for _, source := range deduped {
		lists = append(lists, source)
	}
	sort.Slice(lists, func(i, j int) bool { return lists[i].Name < lists[j].Name })
	return lists, warnings
}

func normalizeKeywords(keywords []string, sources []connectors.Source) []string {
	normalized := make([]string, 0, len(keywords))
	for _, value := range keywords {
		normalized = append(normalized, splitKeywords(value)...)
	}
	for _, source := range sources {
		sourceType := strings.ToLower(strings.TrimSpace(source.SourceType))
		if sourceType != "keyword" && sourceType != "keywords" {
			continue
		}
		normalized = append(normalized, splitKeywords(source.Identifier)...)
	}
	return dedupeKeywords(normalized)
}

func splitKeywords(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed == "" {
			continue
		}
		keywords = append(keywords, trimmed)
	}
	return keywords
}

func dedupeKeywords(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func containsAny(text string, keywords []string) bool {
	if text == "" {
		return false
	}
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func normalizeListIdentifier(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "top", "topstories", "topstory":
		return "top", true
	case "best", "beststories", "beststory":
		return "best", true
	case "new", "newstories", "newstory":
		return "new", true
	default:
		return "", false
	}
}

func (c *Connector) fetchListIDs(ctx context.Context, list string) ([]int, error) {
	endpoint, ok := listEndpoints[list]
	if !ok {
		return nil, fmt.Errorf("unknown list %s", list)
	}
	url := fmt.Sprintf("%s/%s.json", c.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	return ids, nil
}

func (c *Connector) fetchItem(ctx context.Context, id int) (hnItem, error) {
	url := fmt.Sprintf("%s/item/%d.json", c.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return hnItem{}, err
	}
	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return hnItem{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return hnItem{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	var item hnItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return hnItem{}, fmt.Errorf("decode item: %w", err)
	}
	return item, nil
}

type hnItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Text        string `json:"text"`
	Time        int64  `json:"time"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Deleted     bool   `json:"deleted"`
	Dead        bool   `json:"dead"`
}

func stripHTML(value string) string {
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))

	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}
