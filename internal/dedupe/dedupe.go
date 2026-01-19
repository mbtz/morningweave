package dedupe

import (
	"strings"
	"unicode"

	"morningweave/internal/connectors"
)

const DefaultTitleSimilarityThreshold = 0.82

// SourceLink captures the per-platform URL for a merged item.
type SourceLink struct {
	Source connectors.SourceRef
	URL    string
}

// MergedItem represents a deduped item with combined sources.
type MergedItem struct {
	Item         connectors.Item
	Sources      []SourceLink
	CanonicalURL string
}

// Dedupe merges items by canonical URL first, then by fuzzy title similarity.
func Dedupe(items []connectors.Item) []MergedItem {
	merged := make([]MergedItem, 0, len(items))
	canonicalIndex := map[string]int{}

	for _, item := range items {
		canonical := CanonicalizeURL(item.URL)
		if canonical != "" {
			if idx, ok := canonicalIndex[canonical]; ok {
				mergeItem(&merged[idx], item, canonical)
				continue
			}
		}

		matchIdx := -1
		for idx := range merged {
			if TitlesSimilar(item.Title, merged[idx].Item.Title, DefaultTitleSimilarityThreshold) {
				matchIdx = idx
				break
			}
		}

		if matchIdx >= 0 {
			mergeItem(&merged[matchIdx], item, canonical)
			if canonical != "" {
				canonicalIndex[canonical] = matchIdx
			}
			continue
		}

		merged = append(merged, newMerged(item, canonical))
		if canonical != "" {
			canonicalIndex[canonical] = len(merged) - 1
		}
	}

	return merged
}

func newMerged(item connectors.Item, canonical string) MergedItem {
	m := MergedItem{
		Item:         item,
		CanonicalURL: canonical,
	}
	m.Sources = appendSource(m.Sources, SourceLink{Source: item.Source, URL: item.URL})
	return m
}

func mergeItem(target *MergedItem, incoming connectors.Item, canonical string) {
	if canonical != "" && target.CanonicalURL == "" {
		target.CanonicalURL = canonical
	}
	target.Sources = appendSource(target.Sources, SourceLink{Source: incoming.Source, URL: incoming.URL})
	if engagementScore(incoming.Engagement) > engagementScore(target.Item.Engagement) {
		target.Item = incoming
	}
}

func engagementScore(eng connectors.Engagement) int {
	return eng.Score + eng.Comments + eng.Likes + eng.Reposts + eng.Views
}

func appendSource(existing []SourceLink, link SourceLink) []SourceLink {
	if link.URL == "" && isEmptySource(link.Source) {
		return existing
	}
	key := sourceKey(link)
	for _, current := range existing {
		if sourceKey(current) == key {
			return existing
		}
	}
	return append(existing, link)
}

func sourceKey(link SourceLink) string {
	canonical := CanonicalizeURL(link.URL)
	return link.Source.Platform + "|" + link.Source.SourceType + "|" + link.Source.Identifier + "|" + canonical
}

func isEmptySource(source connectors.SourceRef) bool {
	return source.Platform == "" && source.SourceType == "" && source.Identifier == ""
}

// TitlesSimilar reports whether two titles are similar enough to be merged.
func TitlesSimilar(a string, b string, threshold float64) bool {
	if threshold <= 0 {
		threshold = DefaultTitleSimilarityThreshold
	}
	return TitleSimilarity(a, b) >= threshold
}

// TitleSimilarity returns a 0..1 similarity score between titles.
func TitleSimilarity(a string, b string) float64 {
	left := normalizeTitle(a)
	right := normalizeTitle(b)
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 1
	}

	dice := diceCoefficient(left, right)
	jaccard := tokenJaccard(left, right)
	return 0.6*dice + 0.4*jaccard
}

func normalizeTitle(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	var b strings.Builder
	b.Grow(len(lower))
	lastSpace := false
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func tokenJaccard(a string, b string) float64 {
	left := strings.Fields(a)
	right := strings.Fields(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	leftSet := map[string]struct{}{}
	for _, token := range left {
		leftSet[token] = struct{}{}
	}
	rightSet := map[string]struct{}{}
	for _, token := range right {
		rightSet[token] = struct{}{}
	}
	intersection := 0
	union := len(leftSet)
	for token := range rightSet {
		if _, ok := leftSet[token]; ok {
			intersection++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func diceCoefficient(a string, b string) float64 {
	if a == b {
		return 1
	}
	left := []rune(a)
	right := []rune(b)
	if len(left) < 2 || len(right) < 2 {
		return 0
	}

	leftBigrams := map[string]int{}
	for i := 0; i < len(left)-1; i++ {
		bigram := string(left[i : i+2])
		leftBigrams[bigram]++
	}

	matches := 0
	for i := 0; i < len(right)-1; i++ {
		bigram := string(right[i : i+2])
		if count, ok := leftBigrams[bigram]; ok && count > 0 {
			matches++
			leftBigrams[bigram] = count - 1
		}
	}

	total := (len(left) - 1) + (len(right) - 1)
	if total == 0 {
		return 0
	}
	return (2.0 * float64(matches)) / float64(total)
}
