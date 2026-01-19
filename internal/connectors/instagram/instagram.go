package instagram

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
	defaultBaseURL  = "https://graph.facebook.com/v19.0"
	defaultMaxItems = 50
	titleMaxRunes   = 80
)

// Credentials define the secret payload needed for Instagram Graph API access.
type Credentials struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
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
		case "access_token", "token", "bearer", "ig_token":
			creds.AccessToken = value
		case "user_id", "userid", "ig_user_id":
			creds.UserID = value
		}
	}

	if creds.isEmpty() {
		return Credentials{}, errors.New("credentials payload could not be parsed")
	}
	return creds, nil
}

func (c Credentials) isEmpty() bool {
	return strings.TrimSpace(c.AccessToken) == "" && strings.TrimSpace(c.UserID) == ""
}

func (c Credentials) missingRequired() []string {
	if strings.TrimSpace(c.AccessToken) == "" {
		return []string{"access_token"}
	}
	return nil
}

// Connector implements the Instagram Graph API integration.
type Connector struct {
	client  *connectors.HTTPClient
	baseURL string
	now     func() time.Time
	creds   Credentials
	userID  string
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

// WithCredentials supplies the credentials used for API calls.
func WithCredentials(creds Credentials) Option {
	return func(c *Connector) {
		c.creds = creds
		c.userID = strings.TrimSpace(creds.UserID)
	}
}

// New constructs an Instagram connector with defaults applied.
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

// Platform returns the platform key for Instagram.
func (c *Connector) Platform() string {
	return "instagram"
}

// Requirements describes Instagram Graph API auth requirements.
func (c *Connector) Requirements() connectors.Requirements {
	return connectors.Requirements{
		Auth: connectors.AuthRequirements{
			Required: true,
			Scopes:   []string{"instagram_basic", "pages_show_list", "instagram_manage_insights"},
			Notes:    "Use an Instagram Business/Creator token from the Graph API.",
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
	return connectors.Status{Auth: connectors.AuthStatus{Configured: true}}, nil
}

// Fetch retrieves items from configured account and hashtag sources.
func (c *Connector) Fetch(ctx context.Context, req connectors.FetchRequest) (connectors.FetchResult, error) {
	result := connectors.FetchResult{}

	if missing := c.creds.missingRequired(); len(missing) > 0 {
		return result, connectors.ErrAuthMissing
	}

	sources, warnings := normalizeSources(req.Sources)
	result.Warnings = append(result.Warnings, warnings...)
	if len(sources) == 0 {
		if len(req.Keywords) > 0 {
			result.Warnings = append(result.Warnings, "instagram: no sources configured; keywords ignored")
		} else {
			result.Warnings = append(result.Warnings, "instagram: no sources configured")
		}
		return result, nil
	}

	keywords := normalizeKeywords(req.Keywords)
	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}

	needsUserID := needsUserLookup(sources)
	if needsUserID {
		userID, err := c.ensureUserID(ctx)
		if err != nil {
			return result, err
		}
		c.userID = userID
	}

	for _, source := range sources {
		var items []connectors.Item
		var warn []string
		var err error
		if source.SourceType == "accounts" {
			items, warn, err = c.fetchAccount(ctx, source, req, maxItems, keywords)
		} else {
			items, warn, err = c.fetchHashtag(ctx, source, req, maxItems, keywords)
		}
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("instagram: %s fetch failed: %v", source.SourceType, err))
			continue
		}
		result.Warnings = append(result.Warnings, warn...)
		result.Items = append(result.Items, items...)
	}

	return result, nil
}

type mediaItem struct {
	ID             string `json:"id"`
	Caption        string `json:"caption"`
	MediaType      string `json:"media_type"`
	MediaURL       string `json:"media_url"`
	Permalink      string `json:"permalink"`
	Timestamp      string `json:"timestamp"`
	LikeCount      int    `json:"like_count"`
	CommentsCount  int    `json:"comments_count"`
	VideoViewCount int    `json:"video_view_count"`
	ViewCount      int    `json:"view_count"`
	PlayCount      int    `json:"play_count"`
}

type mediaListResponse struct {
	Data []mediaItem `json:"data"`
}

type businessDiscoveryResponse struct {
	BusinessDiscovery struct {
		Media mediaListResponse `json:"media"`
	} `json:"business_discovery"`
}

type hashtagSearchResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
	Subcode int    `json:"error_subcode"`
}

func (e apiError) Error() string {
	parts := []string{}
	if e.Type != "" {
		parts = append(parts, e.Type)
	}
	if e.Code != 0 {
		parts = append(parts, strconv.Itoa(e.Code))
	}
	if e.Subcode != 0 {
		parts = append(parts, fmt.Sprintf("subcode %d", e.Subcode))
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if len(parts) == 0 {
		return "instagram api error"
	}
	return "instagram api error: " + strings.Join(parts, ": ")
}

func (c *Connector) fetchAccount(ctx context.Context, source connectors.Source, req connectors.FetchRequest, maxItems int, keywords []string) ([]connectors.Item, []string, error) {
	identifier := source.Identifier
	if identifier == "" {
		return nil, nil, nil
	}

	var response mediaListResponse
	var err error
	if isNumeric(identifier) {
		response, err = c.fetchAccountMedia(ctx, identifier, maxItems)
	} else {
		userID := c.userID
		if userID == "" {
			var userErr error
			userID, userErr = c.ensureUserID(ctx)
			if userErr != nil {
				return nil, nil, userErr
			}
		}
		response, err = c.fetchBusinessDiscovery(ctx, userID, identifier, maxItems)
	}
	if err != nil {
		return nil, nil, err
	}

	return mapMediaItems(response.Data, source, req, keywords, maxItems, c.now()), nil, nil
}

func (c *Connector) fetchHashtag(ctx context.Context, source connectors.Source, req connectors.FetchRequest, maxItems int, keywords []string) ([]connectors.Item, []string, error) {
	tag := strings.TrimSpace(source.Identifier)
	if tag == "" {
		return nil, nil, nil
	}
	userID := c.userID
	if userID == "" {
		var userErr error
		userID, userErr = c.ensureUserID(ctx)
		if userErr != nil {
			return nil, nil, userErr
		}
	}

	hashtagID, err := c.lookupHashtagID(ctx, userID, tag)
	if err != nil {
		return nil, nil, err
	}
	if hashtagID == "" {
		return nil, []string{fmt.Sprintf("instagram: hashtag %q not found", tag)}, nil
	}

	response, err := c.fetchHashtagMedia(ctx, hashtagID, userID, maxItems)
	if err != nil {
		return nil, nil, err
	}
	return mapMediaItems(response.Data, source, req, keywords, maxItems, c.now()), nil, nil
}

func (c *Connector) ensureUserID(ctx context.Context) (string, error) {
	if c.userID != "" {
		return c.userID, nil
	}
	if strings.TrimSpace(c.creds.UserID) != "" {
		c.userID = strings.TrimSpace(c.creds.UserID)
		return c.userID, nil
	}

	params := url.Values{}
	params.Set("fields", "id,username")

	var resp struct {
		ID string `json:"id"`
	}
	if err := c.get(ctx, "/me", params, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.ID) == "" {
		return "", errors.New("instagram: me response missing id")
	}
	c.userID = strings.TrimSpace(resp.ID)
	return c.userID, nil
}

func (c *Connector) lookupHashtagID(ctx context.Context, userID, tag string) (string, error) {
	params := url.Values{}
	params.Set("user_id", userID)
	params.Set("q", tag)

	var resp hashtagSearchResponse
	if err := c.get(ctx, "/ig_hashtag_search", params, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", nil
	}
	return strings.TrimSpace(resp.Data[0].ID), nil
}

func (c *Connector) fetchHashtagMedia(ctx context.Context, hashtagID, userID string, maxItems int) (mediaListResponse, error) {
	params := url.Values{}
	params.Set("user_id", userID)
	params.Set("fields", mediaFields())
	if maxItems > 0 {
		params.Set("limit", strconv.Itoa(maxItems))
	}

	var resp mediaListResponse
	if err := c.get(ctx, "/"+hashtagID+"/recent_media", params, &resp); err != nil {
		return mediaListResponse{}, err
	}
	return resp, nil
}

func (c *Connector) fetchAccountMedia(ctx context.Context, accountID string, maxItems int) (mediaListResponse, error) {
	params := url.Values{}
	params.Set("fields", mediaFields())
	if maxItems > 0 {
		params.Set("limit", strconv.Itoa(maxItems))
	}

	var resp mediaListResponse
	if err := c.get(ctx, "/"+accountID+"/media", params, &resp); err != nil {
		return mediaListResponse{}, err
	}
	return resp, nil
}

func (c *Connector) fetchBusinessDiscovery(ctx context.Context, userID, username string, maxItems int) (mediaListResponse, error) {
	fields := fmt.Sprintf("business_discovery.username(%s){media{%s}}", username, mediaFields())
	params := url.Values{}
	params.Set("fields", fields)
	if maxItems > 0 {
		params.Set("limit", strconv.Itoa(maxItems))
	}

	var resp businessDiscoveryResponse
	if err := c.get(ctx, "/"+userID, params, &resp); err != nil {
		return mediaListResponse{}, err
	}
	return resp.BusinessDiscovery.Media, nil
}

func (c *Connector) get(ctx context.Context, path string, params url.Values, out any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("access_token", strings.TrimSpace(c.creds.AccessToken))

	endpoint := c.baseURL + path
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		if apiErr := parseAPIError(body); apiErr != nil {
			return apiErr
		}
		return fmt.Errorf("instagram api error: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseAPIError(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var envelope struct {
		Error apiError `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	if strings.TrimSpace(envelope.Error.Message) == "" && envelope.Error.Code == 0 && envelope.Error.Type == "" {
		return nil
	}
	return envelope.Error
}

func mediaFields() string {
	return strings.Join([]string{
		"id",
		"caption",
		"media_type",
		"media_url",
		"permalink",
		"timestamp",
		"like_count",
		"comments_count",
		"video_view_count",
		"view_count",
		"play_count",
	}, ",")
}

func mapMediaItems(items []mediaItem, source connectors.Source, req connectors.FetchRequest, keywords []string, maxItems int, now time.Time) []connectors.Item {
	out := []connectors.Item{}
	for _, item := range items {
		mapped, ok := mapMediaItem(item, source, now)
		if !ok {
			continue
		}
		if !req.Since.IsZero() && mapped.Timestamp.Before(req.Since) {
			continue
		}
		if !req.Until.IsZero() && mapped.Timestamp.After(req.Until) {
			continue
		}
		if len(keywords) > 0 {
			text := strings.ToLower(strings.TrimSpace(mapped.Title + " " + mapped.Text))
			if !containsAny(text, keywords) {
				continue
			}
		}
		out = append(out, mapped)
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
	}
	return out
}

func mapMediaItem(item mediaItem, source connectors.Source, now time.Time) (connectors.Item, bool) {
	caption := strings.TrimSpace(item.Caption)
	title := summarizeCaption(caption, item.MediaType)
	if title == "" {
		title = fallbackTitle(item.MediaType)
	}
	text := strings.TrimSpace(strings.ReplaceAll(caption, "\n", " "))
	urlValue := strings.TrimSpace(item.Permalink)
	if urlValue == "" {
		urlValue = strings.TrimSpace(item.MediaURL)
	}
	if urlValue == "" {
		return connectors.Item{}, false
	}

	timestamp := now
	if parsed, ok := parseTimestamp(item.Timestamp); ok {
		timestamp = parsed
	}

	views := maxInt(item.VideoViewCount, item.ViewCount)
	views = maxInt(views, item.PlayCount)

	return connectors.Item{
		Title: title,
		URL:   urlValue,
		Text:  text,
		Engagement: connectors.Engagement{
			Likes:    item.LikeCount,
			Comments: item.CommentsCount,
			Views:    views,
		},
		Timestamp: timestamp,
		Source: connectors.SourceRef{
			Platform:   "instagram",
			SourceType: source.SourceType,
			Identifier: source.Identifier,
		},
	}, true
}

func parseTimestamp(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05Z0700",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func summarizeCaption(caption string, mediaType string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(caption, "\n", " "))
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= titleMaxRunes {
		return trimmed
	}
	return string(runes[:titleMaxRunes-3]) + "..."
}

func fallbackTitle(mediaType string) string {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	switch mt {
	case "video":
		return "Instagram video"
	case "carousel_album":
		return "Instagram carousel"
	case "image":
		return "Instagram image"
	default:
		return "Instagram post"
	}
}

func normalizeSources(sources []connectors.Source) ([]connectors.Source, []string) {
	if len(sources) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]connectors.Source, 0, len(sources))
	warnings := []string{}
	for _, source := range sources {
		sourceType, ok := normalizeSourceType(source.SourceType)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("instagram: unsupported source type %q", source.SourceType))
			continue
		}
		identifier := strings.TrimSpace(source.Identifier)
		if identifier == "" {
			continue
		}
		if sourceType == "accounts" {
			identifier = strings.TrimPrefix(identifier, "@")
		}
		if sourceType == "hashtags" {
			identifier = strings.TrimPrefix(identifier, "#")
		}
		identifier = strings.TrimSpace(identifier)
		if identifier == "" {
			continue
		}
		key := sourceType + ":" + strings.ToLower(identifier)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
	case "accounts", "account", "user", "users":
		return "accounts", true
	case "hashtags", "hashtag", "tags", "tag":
		return "hashtags", true
	default:
		return "", false
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

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func needsUserLookup(sources []connectors.Source) bool {
	for _, source := range sources {
		if source.SourceType == "hashtags" {
			return true
		}
		if source.SourceType == "accounts" && !isNumeric(source.Identifier) {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
