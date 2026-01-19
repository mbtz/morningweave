package ranking

import (
	"sort"
	"strings"
	"time"

	"morningweave/internal/connectors"
)

const defaultMinLanguageConfidence = 0.6

// RankOptions control the ranking pipeline.
type RankOptions struct {
	Keywords              []string
	AllowedLanguages      []string
	MinLanguageConfidence float64
	SourceWeight          func(item connectors.Item) float64
	Now                   time.Time
}

// ScoredItem captures ranking results for an item.
type ScoredItem struct {
	Item               connectors.Item
	Score              float64
	Components         Components
	TagMatch           MatchResult
	Language           string
	LanguageConfidence float64
	SourceWeight       float64
}

// RankItems filters items by language/keywords and ranks them by composite score.
func RankItems(items []connectors.Item, opts RankOptions) []ScoredItem {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	minConfidence := opts.MinLanguageConfidence
	if minConfidence <= 0 {
		minConfidence = defaultMinLanguageConfidence
	}
	weightFn := opts.SourceWeight
	if weightFn == nil {
		weightFn = func(connectors.Item) float64 { return 1.0 }
	}
	allowed := normalizeLanguageSet(opts.AllowedLanguages)

	scored := make([]ScoredItem, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Title + " " + item.Text)
		lang, confidence := DetectLanguage(text)
		if len(allowed) > 0 {
			if _, ok := allowed[lang]; !ok || confidence < minConfidence {
				continue
			}
		}

		match := MatchKeywords(text, opts.Keywords)
		if len(opts.Keywords) > 0 && match.Score == 0 {
			continue
		}

		sourceWeight := weightFn(item)
		components := ScoreComponents(item, match.Score, sourceWeight, now)
		score := CombinedScore(components)

		scored = append(scored, ScoredItem{
			Item:               item,
			Score:              score,
			Components:         components,
			TagMatch:           match,
			Language:           lang,
			LanguageConfidence: confidence,
			SourceWeight:       sourceWeight,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i]
		right := scored[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if !left.Item.Timestamp.Equal(right.Item.Timestamp) {
			return left.Item.Timestamp.After(right.Item.Timestamp)
		}
		return left.Item.Title < right.Item.Title
	})

	return scored
}
