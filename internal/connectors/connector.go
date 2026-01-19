package connectors

import (
	"context"
	"errors"
	"time"
)

// ErrAuthMissing indicates required credentials are not configured.
var ErrAuthMissing = errors.New("connector credentials not configured")

// Requirements describes auth/scope expectations for a connector.
type Requirements struct {
	Auth AuthRequirements
}

// AuthRequirements describes the credentials/scopes needed by a connector.
type AuthRequirements struct {
	Required bool
	Scopes   []string
	Notes    string
}

// AuthStatus captures whether credentials are configured.
type AuthStatus struct {
	Configured bool
	Missing    []string
	Notes      string
}

// RateLimitStatus summarizes the last known rate-limit state.
type RateLimitStatus struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
	Window    time.Duration
}

// FetchRequest describes the items to fetch from a connector.
type FetchRequest struct {
	Sources   []Source
	Keywords  []string
	Languages []string
	Since     time.Time
	Until     time.Time
	MaxItems  int
	PageToken string
}

// FetchResult contains fetched items plus metadata.
type FetchResult struct {
	Items     []Item
	Warnings  []string
	RateLimit *RateLimitStatus
	Page      PageInfo
}

// PageInfo describes pagination state.
type PageInfo struct {
	NextToken string
	HasMore   bool
}

// Source identifies a configured source input for a connector.
type Source struct {
	SourceType string
	Identifier string
	Weight     float64
}

// Status provides connector health metadata for status output.
type Status struct {
	Auth      AuthStatus
	RateLimit *RateLimitStatus
	Warnings  []string
}

// Connector is the interface implemented by each platform integration.
type Connector interface {
	Platform() string
	Requirements() Requirements
	Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
	Status(ctx context.Context) (Status, error)
}
