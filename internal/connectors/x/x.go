package x

import (
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
	defaultBaseURL       = "https://api.twitter.com/2"
	defaultMaxItems      = 50
	minSearchResults     = 10
	minTimelineResults   = 5
	maxTimelineResults   = 100
	maxSearchResults     = 100
	defaultFallbackTitle = 80
)

// Credentials define the secret payload needed for X API access.
type Credentials struct {
	BearerToken string `json:"bearer_token"`
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

	var rawMap map[string]any
	if json.Unmarshal([]byte(trimmed), &rawMap) == nil {
		applyCredentialMap(&creds, rawMap)
		if !creds.isEmpty() {
			return creds, nil
		}
		if notes, ok := rawMap["notesPlain"].(string); ok {
			applyCredentialPairs(&creds, notes)
			if !creds.isEmpty() {
				return creds, nil
			}
		}
		if fields, ok := rawMap["fields"]; ok {
			applyCredentialFields(&creds, fields)
			if !creds.isEmpty() {
				return creds, nil
			}
		}
	}

	applyCredentialPairs(&creds, trimmed)

	if creds.isEmpty() {
		if strings.HasPrefix(strings.ToLower(trimmed), "bearer ") {
			creds.BearerToken = strings.TrimSpace(trimmed[len("bearer "):])
		}
	}

	if creds.isEmpty() {
		if trimmed != "" &&
			!strings.ContainsAny(trimmed, " \t\n\r") &&
			!strings.ContainsAny(trimmed, ":=") &&
			!strings.HasPrefix(trimmed, "{") &&
			!strings.HasPrefix(trimmed, "[") {
			creds.BearerToken = trimmed
		}
	}

	if creds.isEmpty() {
		return Credentials{}, errors.New("credentials payload could not be parsed")
	}
	return creds, nil
}

func (c Credentials) isEmpty() bool {
	return strings.TrimSpace(c.BearerToken) == ""
}

func (c Credentials) missingRequired() []string {
	if strings.TrimSpace(c.BearerToken) == "" {
		return []string{"bearer_token"}
	}
	return nil
}

// Connector implements the X API integration.
type Connector struct {
	client  *connectors.HTTPClient
	baseURL string
	now     func() time.Time
	creds   Credentials
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
		trimmed := strings.TrimSpace(baseURL)
		if trimmed == "" {
			return
		}
		if parsed, err := url.Parse(trimmed); err == nil {
			if parsed.Path == "" || parsed.Path == "/" {
				trimmed = strings.TrimRight(trimmed, "/") + "/2"
			}
		}
		c.baseURL = strings.TrimRight(trimmed, "/")
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

// New constructs an X connector with defaults applied.
func New(opts ...Option) *Connector {
	c := &Connector{
		client:  connectors.NewHTTPClient(connectors.DefaultRetryConfig()),
		baseURL: defaultBaseURL,
		now:     time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Platform returns the platform key for X.
func (c *Connector) Platform() string {
	return "x"
}

// Requirements describes X auth requirements.
func (c *Connector) Requirements() connectors.Requirements {
	return connectors.Requirements{
		Auth: connectors.AuthRequirements{
			Required: true,
			Scopes:   []string{"tweet.read", "users.read"},
			Notes:    "Provide bearer_token from the X API developer portal.",
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

// Fetch retrieves items from configured keyword, user, and list sources.
func (c *Connector) Fetch(ctx context.Context, req connectors.FetchRequest) (connectors.FetchResult, error) {
	result := connectors.FetchResult{}

	if missing := c.creds.missingRequired(); len(missing) > 0 {
		return result, connectors.ErrAuthMissing
	}

	sources, warnings := normalizeSources(req.Sources)
	result.Warnings = append(result.Warnings, warnings...)

	keywords := normalizeKeywords(req.Keywords)
	languages := normalizeLanguages(req.Languages)
	if len(sources) == 0 {
		if len(keywords) > 0 {
			result.Warnings = append(result.Warnings, "x: no sources configured; keywords ignored")
		} else {
			result.Warnings = append(result.Warnings, "x: no sources configured")
		}
		return result, nil
	}

	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}

	for _, source := range sources {
		items, rate, warn, err := c.fetchSource(ctx, source, req, maxItems, keywords, languages)
		if err != nil {
			if apiErr := asAPIError(err); apiErr != nil {
				if hint := tierWarning(apiErr.Status, apiErr.Message); hint != "" {
					result.Warnings = append(result.Warnings, hint)
					continue
				}
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("x: %s %s fetch failed: %v", source.SourceType, source.Identifier, err))
			continue
		}
		result.Items = append(result.Items, items...)
		result.Warnings = append(result.Warnings, warn...)
		if rate != nil {
			result.RateLimit = rate
		}
	}

	return result, nil
}

func (c *Connector) fetchSource(ctx context.Context, source connectors.Source, req connectors.FetchRequest, maxItems int, keywords []string, languages []string) ([]connectors.Item, *connectors.RateLimitStatus, []string, error) {
	switch source.SourceType {
	case "keywords":
		return c.fetchSearch(ctx, source, req, maxItems, keywords, languages)
	case "users":
		return c.fetchUserTimeline(ctx, source, req, maxItems, keywords, languages)
	case "lists":
		return c.fetchListTweets(ctx, source, req, maxItems, keywords, languages)
	default:
		return nil, nil, []string{fmt.Sprintf("x: unsupported source type %q", source.SourceType)}, nil
	}
}

func (c *Connector) fetchSearch(ctx context.Context, source connectors.Source, req connectors.FetchRequest, maxItems int, keywords []string, languages []string) ([]connectors.Item, *connectors.RateLimitStatus, []string, error) {
	query := strings.TrimSpace(source.Identifier)
	if query == "" {
		return nil, nil, []string{"x: empty keyword source"}, nil
	}
	params := defaultTweetParams(maxItems, minSearchResults, maxSearchResults)
	params.Set("query", query)
	applyTimeBounds(params, req)

	payload, rate, err := c.fetchTweets(ctx, "/tweets/search/recent", params)
	if err != nil {
		return nil, rate, nil, err
	}
	items := buildItems(payload, source, "", keywords, languages)
	return items, rate, nil, nil
}

func (c *Connector) fetchUserTimeline(ctx context.Context, source connectors.Source, req connectors.FetchRequest, maxItems int, keywords []string, languages []string) ([]connectors.Item, *connectors.RateLimitStatus, []string, error) {
	username := strings.TrimSpace(source.Identifier)
	if username == "" {
		return nil, nil, []string{"x: empty user source"}, nil
	}
	userID, warn, err := c.fetchUserID(ctx, username)
	if err != nil {
		return nil, nil, warn, err
	}
	params := defaultTweetParams(maxItems, minTimelineResults, maxTimelineResults)
	applyTimeBounds(params, req)
	payload, rate, err := c.fetchTweets(ctx, fmt.Sprintf("/users/%s/tweets", url.PathEscape(userID)), params)
	if err != nil {
		return nil, rate, warn, err
	}
	items := buildItems(payload, source, username, keywords, languages)
	return items, rate, warn, nil
}

func (c *Connector) fetchListTweets(ctx context.Context, source connectors.Source, req connectors.FetchRequest, maxItems int, keywords []string, languages []string) ([]connectors.Item, *connectors.RateLimitStatus, []string, error) {
	listID := strings.TrimSpace(source.Identifier)
	if listID == "" {
		return nil, nil, []string{"x: empty list source"}, nil
	}
	params := defaultTweetParams(maxItems, minTimelineResults, maxTimelineResults)
	applyTimeBounds(params, req)
	payload, rate, err := c.fetchTweets(ctx, fmt.Sprintf("/lists/%s/tweets", url.PathEscape(listID)), params)
	if err != nil {
		return nil, rate, nil, err
	}
	items := buildItems(payload, source, "", keywords, languages)
	return items, rate, nil, nil
}

func defaultTweetParams(maxItems int, minResults int, maxResults int) url.Values {
	params := url.Values{}
	params.Set("tweet.fields", "created_at,lang,public_metrics")
	params.Set("expansions", "author_id")
	params.Set("user.fields", "username")

	if maxItems > 0 {
		limit := maxItems
		if limit < minResults {
			limit = minResults
		}
		if limit > maxResults {
			limit = maxResults
		}
		params.Set("max_results", strconv.Itoa(limit))
	}
	return params
}

func applyTimeBounds(params url.Values, req connectors.FetchRequest) {
	if !req.Since.IsZero() {
		params.Set("start_time", req.Since.UTC().Format(time.RFC3339))
	}
	if !req.Until.IsZero() {
		params.Set("end_time", req.Until.UTC().Format(time.RFC3339))
	}
}

func (c *Connector) fetchUserID(ctx context.Context, username string) (string, []string, error) {
	path := fmt.Sprintf("/users/by/username/%s", url.PathEscape(username))
	params := url.Values{}
	params.Set("user.fields", "id,username")
	resp, body, _, err := c.doRequest(ctx, http.MethodGet, path, params)
	if err != nil {
		return "", nil, err
	}
	var payload userLookupResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", nil, fmt.Errorf("decode response: %w", err)
		}
	}
	if resp.StatusCode >= 400 {
		message := formatUserError(payload)
		if message == "" {
			message = strings.TrimSpace(resp.Status)
		}
		return "", nil, apiResponseError{Status: resp.StatusCode, Message: message}
	}
	id := strings.TrimSpace(payload.Data.ID)
	if id == "" {
		return "", []string{fmt.Sprintf("x: user %s missing id", username)}, fmt.Errorf("user %s missing id", username)
	}
	return id, nil, nil
}

func (c *Connector) fetchTweets(ctx context.Context, path string, params url.Values) (apiResponse, *connectors.RateLimitStatus, error) {
	payload, rate, err := c.fetchJSON(ctx, path, params)
	return payload, rate, err
}

func (c *Connector) fetchJSON(ctx context.Context, path string, params url.Values) (apiResponse, *connectors.RateLimitStatus, error) {
	resp, body, rate, err := c.doRequest(ctx, http.MethodGet, path, params)
	if err != nil {
		return apiResponse{}, rate, err
	}
	var payload apiResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return apiResponse{}, rate, fmt.Errorf("decode response: %w", err)
		}
	}

	if resp.StatusCode >= 400 {
		message := formatAPIError(payload)
		if message == "" {
			message = strings.TrimSpace(resp.Status)
		}
		return payload, rate, apiResponseError{Status: resp.StatusCode, Message: message}
	}

	return payload, rate, nil
}

func (c *Connector) doRequest(ctx context.Context, method string, path string, params url.Values) (*http.Response, []byte, *connectors.RateLimitStatus, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil, nil, errors.New("path required")
	}
	urlPath := strings.TrimRight(c.baseURL, "/") + path
	if len(params) > 0 {
		urlPath = urlPath + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, urlPath, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.creds.BearerToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, parseRateLimit(resp, c.now()), err
	}
	return resp, body, parseRateLimit(resp, c.now()), nil
}

type apiResponse struct {
	Data     []tweet     `json:"data"`
	Includes includes    `json:"includes"`
	Meta     meta        `json:"meta"`
	Errors   []apiError  `json:"errors"`
	Title    string      `json:"title"`
	Detail   string      `json:"detail"`
	Type     string      `json:"type"`
	Status   interface{} `json:"status"`
}

type apiResponseError struct {
	Status  int
	Message string
}

func (e apiResponseError) Error() string {
	return e.Message
}

type userLookupResponse struct {
	Data   user        `json:"data"`
	Errors []apiError  `json:"errors"`
	Title  string      `json:"title"`
	Detail string      `json:"detail"`
	Type   string      `json:"type"`
	Status interface{} `json:"status"`
}

type includes struct {
	Users []user `json:"users"`
}

type meta struct {
	NextToken   string `json:"next_token"`
	ResultCount int    `json:"result_count"`
}

type tweet struct {
	ID            string          `json:"id"`
	Text          string          `json:"text"`
	CreatedAt     time.Time       `json:"created_at"`
	AuthorID      string          `json:"author_id"`
	Lang          string          `json:"lang"`
	PublicMetrics publicMetrics   `json:"public_metrics"`
	Entities      tweetEntities   `json:"entities"`
	Attachments   tweetAttachment `json:"attachments"`
}

type publicMetrics struct {
	RetweetCount    int `json:"retweet_count"`
	ReplyCount      int `json:"reply_count"`
	LikeCount       int `json:"like_count"`
	QuoteCount      int `json:"quote_count"`
	ImpressionCount int `json:"impression_count"`
}

type tweetEntities struct {
	URLs []tweetURL `json:"urls"`
}

type tweetURL struct {
	ExpandedURL string `json:"expanded_url"`
}

type tweetAttachment struct {
	MediaKeys []string `json:"media_keys"`
}

type user struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type apiError struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Type   string `json:"type"`
}

func formatAPIError(payload apiResponse) string {
	if len(payload.Errors) > 0 {
		parts := make([]string, 0, len(payload.Errors))
		for _, err := range payload.Errors {
			msg := strings.TrimSpace(err.Detail)
			if msg == "" {
				msg = strings.TrimSpace(err.Title)
			}
			if msg == "" {
				msg = strings.TrimSpace(err.Type)
			}
			if msg != "" {
				parts = append(parts, msg)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}

	if payload.Detail != "" {
		return strings.TrimSpace(payload.Detail)
	}
	if payload.Title != "" {
		return strings.TrimSpace(payload.Title)
	}
	if payload.Type != "" {
		return strings.TrimSpace(payload.Type)
	}
	return ""
}

func formatUserError(payload userLookupResponse) string {
	if len(payload.Errors) > 0 {
		parts := make([]string, 0, len(payload.Errors))
		for _, err := range payload.Errors {
			msg := strings.TrimSpace(err.Detail)
			if msg == "" {
				msg = strings.TrimSpace(err.Title)
			}
			if msg == "" {
				msg = strings.TrimSpace(err.Type)
			}
			if msg != "" {
				parts = append(parts, msg)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}

	if payload.Detail != "" {
		return strings.TrimSpace(payload.Detail)
	}
	if payload.Title != "" {
		return strings.TrimSpace(payload.Title)
	}
	if payload.Type != "" {
		return strings.TrimSpace(payload.Type)
	}
	return ""
}

func asAPIError(err error) *apiResponseError {
	var apiErr apiResponseError
	if errors.As(err, &apiErr) {
		return &apiErr
	}
	return nil
}

func tierWarning(status int, message string) string {
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		return ""
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "client-not-enrolled") || strings.Contains(lower, "insufficient") || strings.Contains(lower, "tier") || strings.Contains(lower, "limited") || strings.Contains(lower, "access") {
		return fmt.Sprintf("x: api tier/access issue: %s", message)
	}
	if status == http.StatusUnauthorized {
		return fmt.Sprintf("x: credentials rejected: %s", message)
	}
	return ""
}

func buildItems(payload apiResponse, source connectors.Source, knownUsername string, keywords []string, languages []string) []connectors.Item {
	usernames := map[string]string{}
	for _, user := range payload.Includes.Users {
		id := strings.TrimSpace(user.ID)
		name := strings.TrimSpace(user.Username)
		if id != "" && name != "" {
			usernames[id] = name
		}
	}

	items := make([]connectors.Item, 0, len(payload.Data))
	for _, tweet := range payload.Data {
		text := strings.TrimSpace(strings.ReplaceAll(tweet.Text, "\n", " "))
		if text == "" {
			continue
		}
		if len(languages) > 0 {
			lang := strings.TrimSpace(strings.ToLower(tweet.Lang))
			if lang != "" && !containsLanguage(lang, languages) {
				continue
			}
		}
		searchText := strings.ToLower(text)
		if !containsAny(searchText, keywords) {
			continue
		}

		username := strings.TrimSpace(knownUsername)
		if username == "" {
			username = usernames[strings.TrimSpace(tweet.AuthorID)]
		}
		url := buildTweetURL(username, tweet.ID)
		items = append(items, connectors.Item{
			Title: summarizeTitle(text),
			URL:   url,
			Text:  text,
			Engagement: connectors.Engagement{
				Score:    tweet.PublicMetrics.LikeCount + tweet.PublicMetrics.RetweetCount + tweet.PublicMetrics.QuoteCount,
				Comments: tweet.PublicMetrics.ReplyCount,
				Likes:    tweet.PublicMetrics.LikeCount,
				Reposts:  tweet.PublicMetrics.RetweetCount + tweet.PublicMetrics.QuoteCount,
				Views:    tweet.PublicMetrics.ImpressionCount,
			},
			Timestamp: tweet.CreatedAt,
			Source: connectors.SourceRef{
				Platform:   "x",
				SourceType: source.SourceType,
				Identifier: source.Identifier,
			},
		})
	}

	return items
}

func buildTweetURL(username string, id string) string {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return ""
	}
	trimmedUser := strings.TrimSpace(username)
	if trimmedUser == "" {
		return fmt.Sprintf("https://x.com/i/web/status/%s", trimmedID)
	}
	return fmt.Sprintf("https://x.com/%s/status/%s", trimmedUser, trimmedID)
}

func summarizeTitle(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= defaultFallbackTitle {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= defaultFallbackTitle {
		return trimmed
	}
	return string(runes[:defaultFallbackTitle-3]) + "..."
}

func normalizeSources(sources []connectors.Source) ([]connectors.Source, []string) {
	if len(sources) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]connectors.Source, 0, len(sources))
	warnings := []string{}
	for _, source := range sources {
		typeKey := strings.ToLower(strings.TrimSpace(source.SourceType))
		id := strings.TrimSpace(source.Identifier)
		if typeKey == "" || id == "" {
			continue
		}
		switch typeKey {
		case "keywords", "users", "lists":
			key := typeKey + ":" + strings.ToLower(id)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, connectors.Source{SourceType: typeKey, Identifier: id, Weight: source.Weight})
		default:
			warnings = append(warnings, fmt.Sprintf("x: unsupported source type %q", source.SourceType))
		}
	}
	return out, warnings
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

func normalizeLanguages(languages []string) []string {
	if len(languages) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range languages {
		clean := strings.ToLower(strings.TrimSpace(raw))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func containsLanguage(lang string, allowed []string) bool {
	for _, candidate := range allowed {
		if lang == candidate {
			return true
		}
	}
	return false
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
	return strings.Split(value, ",")
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

func applyCredentialMap(creds *Credentials, raw map[string]any) {
	if creds == nil || len(raw) == 0 {
		return
	}
	for key, value := range raw {
		applyCredentialValue(creds, key, value)
		if !creds.isEmpty() {
			return
		}
	}
}

func applyCredentialFields(creds *Credentials, fields any) {
	if creds == nil {
		return
	}
	items, ok := fields.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		field, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := ""
		if label, ok := field["label"].(string); ok {
			key = label
		} else if name, ok := field["name"].(string); ok {
			key = name
		} else if id, ok := field["id"].(string); ok {
			key = id
		}
		if key == "" {
			continue
		}
		applyCredentialValue(creds, key, field["value"])
		if !creds.isEmpty() {
			return
		}
	}
}

func applyCredentialPairs(creds *Credentials, value string) {
	if creds == nil {
		return
	}
	for _, part := range splitPairs(value) {
		key, val, ok := splitPair(part)
		if !ok {
			continue
		}
		applyCredentialValue(creds, key, val)
		if !creds.isEmpty() {
			return
		}
	}
}

func applyCredentialValue(creds *Credentials, key string, value any) {
	if creds == nil {
		return
	}
	normalized := normalizeCredentialKey(key)
	token := normalizeCredentialToken(value)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("bearer "):])
	}
	if token == "" {
		return
	}
	switch normalized {
	case "bearer_token", "bearer", "token", "access_token", "x_api_key", "x_ap_key", "xapikey", "api_key", "apikey":
		creds.BearerToken = token
	}
}

func normalizeCredentialKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

func normalizeCredentialToken(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	case map[string]any:
		if inner, ok := v["value"]; ok {
			return normalizeCredentialToken(inner)
		}
		if inner, ok := v["text"]; ok {
			return normalizeCredentialToken(inner)
		}
		if inner, ok := v["token"]; ok {
			return normalizeCredentialToken(inner)
		}
	}
	return ""
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
	limit, okLimit := parseRateLimitInt(resp.Header.Get("x-rate-limit-limit"))
	remaining, okRemaining := parseRateLimitInt(resp.Header.Get("x-rate-limit-remaining"))
	resetEpoch, okReset := parseRateLimitInt(resp.Header.Get("x-rate-limit-reset"))
	if !okLimit && !okRemaining && !okReset {
		return nil
	}
	reset := time.Time{}
	window := time.Duration(0)
	if resetEpoch > 0 {
		reset = time.Unix(int64(resetEpoch), 0)
		if reset.After(now) {
			window = reset.Sub(now)
		}
	}
	return &connectors.RateLimitStatus{
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   reset,
		Window:    window,
	}
}

func parseRateLimitInt(value string) (int, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
