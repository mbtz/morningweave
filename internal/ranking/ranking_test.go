package ranking

import (
	"math"
	"testing"
	"time"

	"morningweave/internal/connectors"
)

func TestStem(t *testing.T) {
	cases := map[string]string{
		"running":  "run",
		"runs":     "run",
		"learned":  "learn",
		"learning": "learn",
	}
	for input, expected := range cases {
		if got := Stem(input); got != expected {
			t.Fatalf("expected %q to stem to %q, got %q", input, expected, got)
		}
	}
}

func TestMatchKeywords(t *testing.T) {
	text := "Machine learning models are running quickly."
	keywords := []string{"machine learning", "run"}
	result := MatchKeywords(text, keywords)
	if result.Score < 0.99 {
		t.Fatalf("expected full keyword match, got score %0.2f", result.Score)
	}
	if len(result.Matched) != 2 {
		t.Fatalf("expected 2 matched keywords, got %d", len(result.Matched))
	}
}

func TestRecencyScore(t *testing.T) {
	now := time.Now()
	if RecencyScore(now, now, DefaultHalfLife) != 1 {
		t.Fatalf("expected 1 for fresh item")
	}
	halfLifeAgo := now.Add(-DefaultHalfLife)
	score := RecencyScore(halfLifeAgo, now, DefaultHalfLife)
	if math.Abs(score-0.5) > 0.05 {
		t.Fatalf("expected ~0.5 after half-life, got %0.2f", score)
	}
}

func TestEngagementScoreMonotonic(t *testing.T) {
	low := EngagementScore("reddit", connectors.Engagement{Score: 1, Comments: 1})
	high := EngagementScore("reddit", connectors.Engagement{Score: 50, Comments: 10})
	if high <= low {
		t.Fatalf("expected higher engagement to score higher: low=%0.2f high=%0.2f", low, high)
	}
}

func TestCombinedScore(t *testing.T) {
	components := Components{Tag: 1, Engagement: 1, Recency: 1, SourceWeight: 1}
	score := CombinedScore(components)
	if score <= 0.8 {
		t.Fatalf("expected strong combined score, got %0.2f", score)
	}
	if score > 1 {
		t.Fatalf("expected score <= 1, got %0.2f", score)
	}
}
