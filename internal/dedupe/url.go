package dedupe

import (
	"net/url"
	"path"
	"strings"
)

var dropQueryKeys = map[string]struct{}{
	"fbclid":       {},
	"gclid":        {},
	"igshid":       {},
	"mc_cid":       {},
	"mc_eid":       {},
	"mkt_tok":      {},
	"ref":          {},
	"ref_src":      {},
	"ref_url":      {},
	"referrer":     {},
	"utm":          {},
	"utm_campaign": {},
	"utm_content":  {},
	"utm_medium":   {},
	"utm_name":     {},
	"utm_referrer": {},
	"utm_source":   {},
	"utm_term":     {},
	"yclid":        {},
	"_hsenc":       {},
	"_hsmi":        {},
}

// CanonicalizeURL normalizes a URL for dedupe keys.
func CanonicalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		fallback, fallbackErr := url.Parse("https://" + trimmed)
		if fallbackErr != nil {
			return trimmed
		}
		parsed = fallback
	}

	originalScheme := strings.ToLower(parsed.Scheme)
	scheme := originalScheme
	if scheme == "http" || scheme == "https" {
		scheme = "https"
	}

	host := strings.ToLower(parsed.Hostname())
	if strings.HasPrefix(host, "www.") {
		host = strings.TrimPrefix(host, "www.")
	}

	port := parsed.Port()
	if port != "" {
		if (originalScheme == "https" && port == "443") || (originalScheme == "http" && port == "80") {
			port = ""
		}
	}
	if port != "" {
		host = host + ":" + port
	}

	cleanPath := path.Clean(parsed.Path)
	if cleanPath == "." {
		cleanPath = "/"
	}

	query := parsed.Query()
	for key := range query {
		if shouldDropQueryKey(key) {
			delete(query, key)
			continue
		}
		if len(query[key]) == 0 {
			delete(query, key)
		}
	}

	rawQuery := ""
	if len(query) > 0 {
		rawQuery = query.Encode()
	}

	return (&url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     cleanPath,
		RawQuery: rawQuery,
	}).String()
}

func shouldDropQueryKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "utm_") {
		return true
	}
	_, ok := dropQueryKeys[lower]
	return ok
}
