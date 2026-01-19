package reddit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"morningweave/internal/connectors"
)

const (
	defaultAuthURL  = "https://www.reddit.com/api/v1/access_token"
	defaultBaseURL  = "https://oauth.reddit.com"
	defaultMaxItems = 50
)

// Credentials define the secret payload needed for Reddit API access.
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	UserAgent    string `json:"user_agent"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

// ParseCredentials parses a JSON or key-value credential payload.
func ParseCredentials(raw string) (Credentials, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Credentials{}, errors.New("credentials payload is empty")
	}

	var creds Credentials
	if json.Unmarshal([]byte(trimmed), &creds) == nil {
		if !creds.isEmpty() {
			return creds, nil
		}
	}

	for _, part := range splitPairs(trimmed) {
		key, value, ok := splitPair(part)
		if !ok {
			continue
		}
		switch key {
		case "client_id", "clientid", "id":
			creds.ClientID = value
		case "client_secret", "clientsecret", "secret":
			creds.ClientSecret = value
		case "user_agent", "useragent", "agent":
			creds.UserAgent = value
		case "username", "user":
			creds.Username = value
		case "password", "pass":
			creds.Password = value
		}
	}

	if creds.isEmpty() {
		return Credentials{}, errors.New("credentials payload could not be parsed")
	}
	return creds, nil
}

func (c Credentials) isEmpty() bool {
	return strings.TrimSpace(c.ClientID) == "" &&
		strings.TrimSpace(c.ClientSecret) == "" &&
		strings.TrimSpace(c.UserAgent) == "" &&
		strings.TrimSpace(c.Username) == "" &&
		strings.TrimSpace(c.Password) == ""
}

func (c Credentials) missingRequired() []string {
	missing := []string{}
	if strings.TrimSpace(c.ClientID) == "" {
		missing = append(missing, "client_id")
	}
	if strings.TrimSpace(c.UserAgent) == "" {
		missing = append(missing, "user_agent")
	}
	return missing
}

// Connector implements the Reddit API integration.
type Connector struct {
	client         *connectors.HTTPClient
	baseURL        string
	authURL        string
	now            func() time.Time
	creds          Credentials
	accessTokenVal string
	accessTokenExp time.Time
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

// WithAuthURL overrides the OAuth token endpoint (primarily for tests).
func WithAuthURL(authURL string) Option {
	return func(c *Connector) {
		if strings.TrimSpace(authURL) != "" {
			c.authURL = strings.TrimSpace(authURL)
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

// WithCredentials supplies the credentials used for API calls.
func WithCredentials(creds Credentials) Option {
	return func(c *Connector) {
		c.creds = creds
	}
}

// New constructs a Reddit connector with defaults applied.
func New(opts ...Option) *Connector {
	c := &Connector{
		client:  connectors.NewHTTPClient(connectors.DefaultRetryConfig()),
		baseURL: defaultBaseURL,
		authURL: defaultAuthURL,
		now:     time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Platform returns the platform key for Reddit.
func (c *Connector) Platform() string {
	return "reddit"
}

// Requirements describes Reddit auth requirements.
func (c *Connector) Requirements() connectors.Requirements {
	return connectors.Requirements{
		Auth: connectors.AuthRequirements{
			Required: true,
			Scopes:   []string{"read"},
			Notes:    "Provide client_id, user_agent, and optionally client_secret or username/password.",
		},
	}
}

// Status reports connector status.
func (c *Connector) Status(ctx context.Context) (connectors.Status, error) {
	_ = ctx
	missing := c.creds.missingRequired()
	if len(missing) > 0 {
		return connectors.Status{
			Auth: connectors.AuthStatus{Configured: false, Missing: missing},
		}, nil
	}
	return connectors.Status{
		Auth: connectors.AuthStatus{Configured: true},
	}, nil
}

// Fetch retrieves items from configured subreddit/user/search sources.
func (c *Connector) Fetch(ctx context.Context, req connectors.FetchRequest) (connectors.FetchResult, error) {
	result := connectors.FetchResult{}

	sources, warnings := normalizeSources(req.Sources)
	result.Warnings = append(result.Warnings, warnings...)
	if len(sources) == 0 {
		result.Warnings = append(result.Warnings, "reddit: no sources configured")
		return result, nil
	}

	missing := c.creds.missingRequired()
	if len(missing) > 0 {
		return result, connectors.ErrAuthMissing
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return result, err
	}

	keywords := normalizeKeywords(req.Keywords)
	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}
	limit := maxItems
	if limit > 100 {
		limit = 100
	}

	for _, source := range sources {
		items, rateLimit, warn, err := c.fetchSource(ctx, token, source, req, limit, keywords)
		if err != nil {
			return result, err
		}
		if rateLimit != nil {
			result.RateLimit = rateLimit
		}
		result.Warnings = append(result.Warnings, warn...)
		result.Items = append(result.Items, items...)
	}

	return result, nil
}

type listing struct {
	Data listingData `json:"data"`
}

type listingData struct {
	Children []listingChild `json:"children"`
	After    string         `json:"after"`
}

type listingChild struct {
	Data postData `json:"data"`
}

type postData struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	SelfText    string  `json:"selftext"`
	Permalink   string  `json:"permalink"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (c *Connector) fetchSource(ctx context.Context, token string, source connectors.Source, req connectors.FetchRequest, limit int, keywords []string) ([]connectors.Item, *connectors.RateLimitStatus, []string, error) {
	var warnings []string
	endpoint, ok := sourceEndpoint(source)
	if !ok {
		warnings = append(warnings, fmt.Sprintf("reddit: unsupported source type %q", source.SourceType))
		return nil, nil, warnings, nil
	}

	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))
	if req.PageToken != "" {
		params.Set("after", req.PageToken)
	}
	if endpoint.Path == "/search" {
		params.Set("q", source.Identifier)
		params.Set("sort", "new")
		params.Set("type", "link")
	}

	list, rateLimit, err := c.fetchListing(ctx, token, endpoint.Path, params)
	if err != nil {
		return nil, rateLimit, warnings, err
	}

	items := make([]connectors.Item, 0, len(list.Data.Children))
	for _, child := range list.Data.Children {
		item, ok := mapItem(child.Data, source)
		if !ok {
			continue
		}
		if !req.Since.IsZero() && item.Timestamp.Before(req.Since) {
			continue
		}
		if !req.Until.IsZero() && item.Timestamp.After(req.Until) {
			continue
		}
		if len(keywords) > 0 {
			text := strings.ToLower(strings.TrimSpace(item.Title + " " + item.Text))
			if !containsAny(text, keywords) {
				continue
			}
		}
		items = append(items, item)
	}

	return items, rateLimit, warnings, nil
}

func mapItem(data postData, source connectors.Source) (connectors.Item, bool) {
	title := strings.TrimSpace(data.Title)
	if title == "" {
		return connectors.Item{}, false
	}
	text := strings.TrimSpace(strings.ReplaceAll(data.SelfText, "\n", " "))
	urlValue := strings.TrimSpace(data.URL)
	if urlValue == "" {
		permalink := strings.TrimSpace(data.Permalink)
		if permalink != "" {
			urlValue = "https://www.reddit.com" + permalink
		}
	}
	timestamp := time.Unix(int64(data.CreatedUTC), 0)

	return connectors.Item{
		Title: title,
		URL:   urlValue,
		Text:  text,
		Engagement: connectors.Engagement{
			Score:    data.Score,
			Comments: data.NumComments,
		},
		Timestamp: timestamp,
		Source: connectors.SourceRef{
			Platform:   "reddit",
			SourceType: source.SourceType,
			Identifier: source.Identifier,
		},
	}, true
}

func (c *Connector) fetchListing(ctx context.Context, token string, path string, params url.Values) (listing, *connectors.RateLimitStatus, error) {
	endpoint := c.baseURL + path
	if params != nil && len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return listing{}, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", c.creds.UserAgent)

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return listing{}, nil, err
	}
	defer resp.Body.Close()

	rateLimit := parseRateLimit(resp, c.now())
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return listing{}, rateLimit, fmt.Errorf("reddit api error: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}

	var list listing
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return listing{}, rateLimit, err
	}
	return list, rateLimit, nil
}

func (c *Connector) accessToken(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.accessTokenVal) != "" && c.accessTokenExp.After(c.now().Add(-1*time.Minute)) {
		return c.accessTokenVal, nil
	}

	missing := c.creds.missingRequired()
	if len(missing) > 0 {
		return "", connectors.ErrAuthMissing
	}

	form := url.Values{}
	grantType := "client_credentials"
	if strings.TrimSpace(c.creds.Username) != "" && strings.TrimSpace(c.creds.Password) != "" {
		grantType = "password"
		form.Set("username", c.creds.Username)
		form.Set("password", c.creds.Password)
	}
	form.Set("grant_type", grantType)
	form.Set("scope", "read")

	body := []byte(form.Encode())
	req, err := http.NewRequest(http.MethodPost, c.authURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.SetBasicAuth(c.creds.ClientID, c.creds.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.creds.UserAgent)

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("reddit auth error: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("reddit auth response missing access_token")
	}

	c.accessTokenVal = payload.AccessToken
	if payload.ExpiresIn > 0 {
		c.accessTokenExp = c.now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	} else {
		c.accessTokenExp = c.now().Add(55 * time.Minute)
	}
	return c.accessTokenVal, nil
}

func normalizeSources(sources []connectors.Source) ([]connectors.Source, []string) {
	if len(sources) == 0 {
		return nil, nil
	}

	warnings := []string{}
	out := make([]connectors.Source, 0, len(sources))
	for _, source := range sources {
		sourceType, ok := normalizeSourceType(source.SourceType)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("reddit: unsupported source type %q", source.SourceType))
			continue
		}
		identifier := strings.TrimSpace(source.Identifier)
		if identifier == "" {
			continue
		}
		out = append(out, connectors.Source{
			SourceType: sourceType,
			Identifier: identifier,
			Weight:     source.Weight,
		})
	}
	return out, warnings
}

func normalizeSourceType(sourceType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "subreddit", "subreddits", "sub":
		return "subreddits", true
	case "user", "users":
		return "users", true
	case "keyword", "keywords", "search", "query":
		return "keywords", true
	default:
		return "", false
	}
}

type redditEndpoint struct {
	Path string
}

func sourceEndpoint(source connectors.Source) (redditEndpoint, bool) {
	identifier := url.PathEscape(strings.TrimSpace(source.Identifier))
	if identifier == "" {
		return redditEndpoint{}, false
	}

	switch source.SourceType {
	case "subreddits":
		return redditEndpoint{Path: "/r/" + identifier + "/new"}, true
	case "users":
		return redditEndpoint{Path: "/user/" + identifier + "/submitted"}, true
	case "keywords":
		return redditEndpoint{Path: "/search"}, true
	default:
		return redditEndpoint{}, false
	}
}

func normalizeKeywords(keywords []string) []string {
	if len(keywords) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range keywords {
		for _, part := range splitCSV(raw) {
			clean := strings.ToLower(strings.TrimSpace(part))
			if clean == "" {
				continue
			}
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	return out
}

func containsAny(text string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	return parts
}

func splitPairs(value string) []string {
	splitter := func(r rune) bool {
		switch r {
		case '\n', ';', ',':
			return true
		default:
			return false
		}
	}
	parts := strings.FieldsFunc(value, splitter)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func splitPair(value string) (string, string, bool) {
	if strings.TrimSpace(value) == "" {
		return "", "", false
	}
	for _, sep := range []string{"=", ":"} {
		if strings.Contains(value, sep) {
			parts := strings.SplitN(value, sep, 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.TrimSpace(parts[1])
			if key == "" || val == "" {
				return "", "", false
			}
			return key, val, true
		}
	}
	return "", "", false
}

func parseRateLimit(resp *http.Response, now time.Time) *connectors.RateLimitStatus {
	if resp == nil {
		return nil
	}
	remaining, ok := parseRateLimitFloat(resp.Header.Get("X-Ratelimit-Remaining"))
	resetSeconds, resetOk := parseRateLimitFloat(resp.Header.Get("X-Ratelimit-Reset"))
	used, _ := parseRateLimitFloat(resp.Header.Get("X-Ratelimit-Used"))
	if !ok && !resetOk && used == 0 {
		return nil
	}

	remainingInt := int(remaining)
	limit := remainingInt
	if used > 0 {
		limit = int(remaining + used)
	}

	reset := time.Time{}
	window := time.Duration(0)
	if resetSeconds > 0 {
		window = time.Duration(resetSeconds) * time.Second
		reset = now.Add(window)
	}

	return &connectors.RateLimitStatus{
		Limit:     limit,
		Remaining: remainingInt,
		ResetAt:   reset,
		Window:    window,
	}
}

func parseRateLimitFloat(value string) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
