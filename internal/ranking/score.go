package ranking

import (
	"time"

	"morningweave/internal/connectors"
)

const (
	weightTag        = 0.45
	weightEngagement = 0.25
	weightRecency    = 0.25
	weightSource     = 0.05
)

type Components struct {
	Tag          float64
	Engagement   float64
	Recency      float64
	SourceWeight float64
}

// ScoreComponents builds scoring components for an item.
func ScoreComponents(item connectors.Item, tagScore float64, sourceWeight float64, now time.Time) Components {
	return Components{
		Tag:          clamp01(tagScore),
		Engagement:   EngagementScore(item.Source.Platform, item.Engagement),
		Recency:      RecencyScore(item.Timestamp, now, DefaultHalfLife),
		SourceWeight: sourceWeight,
	}
}

// CombinedScore combines normalized scoring components into a final 0..1 score.
func CombinedScore(components Components) float64 {
	source := normalizeSourceWeight(components.SourceWeight)
	total := weightTag*clamp01(components.Tag) +
		weightEngagement*clamp01(components.Engagement) +
		weightRecency*clamp01(components.Recency) +
		weightSource*source
	return clamp01(total)
}

func normalizeSourceWeight(weight float64) float64 {
	if weight <= 0 {
		return 0
	}
	return clamp01(weight / 2.0)
}
