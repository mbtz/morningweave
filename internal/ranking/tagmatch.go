package ranking

import (
	"strings"
	"unicode"
)

type MatchResult struct {
	Score   float64
	Matched []string
	Total   int
}

var stemSuffixes = []string{
	"ization",
	"ations",
	"ation",
	"ments",
	"ment",
	"nesses",
	"ness",
	"ingly",
	"edly",
	"ende",
	"ene",
	"ane",
	"ing",
	"ers",
	"er",
	"ed",
	"es",
	"ar",
	"en",
	"et",
	"e",
	"s",
	"t",
	"ly",
}

// Stem reduces a token to a simplified stem for keyword matching.
func Stem(token string) string {
	lower := strings.ToLower(strings.TrimSpace(token))
	if lower == "" {
		return ""
	}
	for _, suffix := range stemSuffixes {
		if strings.HasSuffix(lower, suffix) && len(lower) > len(suffix)+2 {
			stem := lower[:len(lower)-len(suffix)]
			return dropDoubleConsonant(stem)
		}
	}
	return lower
}

func dropDoubleConsonant(stem string) string {
	if len(stem) < 2 {
		return stem
	}
	last := stem[len(stem)-1]
	prev := stem[len(stem)-2]
	if last != prev {
		return stem
	}
	if isVowel(last) {
		return stem
	}
	return stem[:len(stem)-1]
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	default:
		return false
	}
}

// MatchKeywords checks keyword matches against text using stemmed tokens.
func MatchKeywords(text string, keywords []string) MatchResult {
	stemSet := tokenizeStemSet(text)
	result := MatchResult{Total: len(keywords)}
	if len(stemSet) == 0 || len(keywords) == 0 {
		return result
	}

	matches := 0
	for _, keyword := range keywords {
		kwTokens := tokenize(keyword)
		if len(kwTokens) == 0 {
			continue
		}
		if keywordMatches(stemSet, kwTokens) {
			matches++
			result.Matched = append(result.Matched, keyword)
		}
	}

	if matches > 0 {
		result.Score = float64(matches) / float64(len(keywords))
	}
	return result
}

func keywordMatches(stems map[string]struct{}, tokens []string) bool {
	for _, token := range tokens {
		stem := Stem(token)
		if stem == "" {
			return false
		}
		if _, ok := stems[stem]; !ok {
			return false
		}
	}
	return true
}

func tokenizeStemSet(text string) map[string]struct{} {
	stems := map[string]struct{}{}
	for _, token := range tokenize(text) {
		stem := Stem(token)
		if stem == "" {
			continue
		}
		stems[stem] = struct{}{}
	}
	return stems
}

func tokenize(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []string{}
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
	return strings.Fields(b.String())
}
