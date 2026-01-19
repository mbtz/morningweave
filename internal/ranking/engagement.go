package ranking

import "morningweave/internal/connectors"

type EngagementWeights struct {
	Score      float64
	Comments   float64
	Likes      float64
	Reposts    float64
	Views      float64
	Saturation float64
}

var platformEngagement = map[string]EngagementWeights{
	"reddit": {
		Score:      1.0,
		Comments:   2.0,
		Likes:      0.2,
		Reposts:    0.0,
		Views:      0.0,
		Saturation: 150,
	},
	"hn": {
		Score:      1.0,
		Comments:   2.0,
		Likes:      0.0,
		Reposts:    0.0,
		Views:      0.0,
		Saturation: 80,
	},
	"x": {
		Score:      0.0,
		Comments:   1.5,
		Likes:      1.0,
		Reposts:    2.0,
		Views:      0.01,
		Saturation: 500,
	},
	"instagram": {
		Score:      0.0,
		Comments:   3.0,
		Likes:      1.0,
		Reposts:    0.0,
		Views:      0.02,
		Saturation: 800,
	},
}

var defaultEngagement = EngagementWeights{
	Score:      1.0,
	Comments:   1.0,
	Likes:      1.0,
	Reposts:    1.5,
	Views:      0.01,
	Saturation: 200,
}

// EngagementScore normalizes engagement signals to a 0..1 scale.
func EngagementScore(platform string, eng connectors.Engagement) float64 {
	weights := weightsForPlatform(platform)
	raw := weights.Score*float64(eng.Score) +
		weights.Comments*float64(eng.Comments) +
		weights.Likes*float64(eng.Likes) +
		weights.Reposts*float64(eng.Reposts) +
		weights.Views*float64(eng.Views)
	if raw <= 0 {
		return 0
	}
	saturation := weights.Saturation
	if saturation <= 0 {
		saturation = defaultEngagement.Saturation
	}
	return clamp01(raw / (raw + saturation))
}

func weightsForPlatform(platform string) EngagementWeights {
	if weights, ok := platformEngagement[platform]; ok {
		return weights
	}
	return defaultEngagement
}
