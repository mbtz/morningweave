package ranking

import (
	"strings"
	"unicode"

	"morningweave/internal/connectors"
)

const (
	LanguageEnglish   = "en"
	LanguageNorwegian = "no"
)

var englishStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "but": {},
	"by": {}, "for": {}, "from": {}, "has": {}, "have": {}, "he": {}, "her": {}, "his": {},
	"i": {}, "in": {}, "is": {}, "it": {}, "its": {}, "of": {}, "on": {}, "or": {},
	"our": {}, "she": {}, "that": {}, "the": {}, "their": {}, "they": {}, "this": {}, "to": {},
	"was": {}, "we": {}, "were": {}, "with": {}, "you": {}, "your": {},
}

var norwegianStopwords = map[string]struct{}{
	"av": {}, "bare": {}, "ble": {}, "blir": {}, "da": {}, "de": {}, "deg": {}, "den": {},
	"der": {}, "det": {}, "din": {}, "du": {}, "en": {}, "er": {}, "et": {}, "fra": {},
	"har": {}, "hun": {}, "i": {}, "ikke": {}, "jeg": {}, "kan": {}, "med": {}, "men": {},
	"må": {}, "nå": {}, "og": {}, "oss": {}, "på": {}, "seg": {}, "som": {}, "til": {},
	"var": {}, "vi": {}, "vår": {}, "våre": {},
}

// DetectLanguage returns a best-effort language tag and confidence score.
// The detector is optimized for en/no by counting stopwords and Nordic characters.
func DetectLanguage(text string) (string, float64) {
	tokens := tokenizeLetters(text)
	if len(tokens) == 0 {
		return "unknown", 0
	}

	enScore := 0
	noScore := 0
	for _, token := range tokens {
		if containsNordicRune(token) {
			noScore += 2
		}
		if _, ok := englishStopwords[token]; ok {
			enScore++
		}
		if _, ok := norwegianStopwords[token]; ok {
			noScore++
		}
	}

	total := enScore + noScore
	if total < 2 {
		return "unknown", 0
	}

	lang := LanguageEnglish
	best := enScore
	if noScore > enScore {
		lang = LanguageNorwegian
		best = noScore
	}

	confidence := float64(best) / float64(total)
	return lang, confidence
}

// FilterItemsByLanguage keeps items that match allowed language tags with sufficient confidence.
func FilterItemsByLanguage(items []connectors.Item, allowed []string, minConfidence float64) []connectors.Item {
	allowSet := normalizeLanguageSet(allowed)
	if len(allowSet) == 0 {
		return items
	}

	filtered := make([]connectors.Item, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Title + " " + item.Text)
		lang, confidence := DetectLanguage(text)
		if _, ok := allowSet[lang]; ok && confidence >= minConfidence {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func normalizeLanguageSet(allowed []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, lang := range allowed {
		switch strings.TrimSpace(strings.ToLower(lang)) {
		case "en", "eng", "english":
			set[LanguageEnglish] = struct{}{}
		case "no", "nb", "nn", "norwegian", "norsk", "bokmal", "bokmål", "nynorsk":
			set[LanguageNorwegian] = struct{}{}
		}
	}
	return set
}

func tokenizeLetters(text string) []string {
	lower := strings.ToLower(text)
	return strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
}

func containsNordicRune(token string) bool {
	for _, r := range token {
		switch r {
		case 'å', 'ø', 'æ':
			return true
		}
	}
	return false
}
