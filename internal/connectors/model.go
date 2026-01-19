package connectors

import "time"

// Item is the normalized unit returned by platform connectors.
type Item struct {
	Title      string
	URL        string
	Text       string
	Engagement Engagement
	Timestamp  time.Time
	Source     SourceRef
}

// Engagement captures common engagement signals across platforms.
type Engagement struct {
	Score    int
	Comments int
	Likes    int
	Reposts  int
	Views    int
}

// SourceRef identifies the platform and source that produced an item.
type SourceRef struct {
	Platform   string
	SourceType string
	Identifier string
}
