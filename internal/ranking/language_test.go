package ranking

import (
	"testing"

	"morningweave/internal/connectors"
)

func TestDetectLanguageEnglish(t *testing.T) {
	lang, confidence := DetectLanguage("This is a simple test of the language filter.")
	if lang != LanguageEnglish {
		t.Fatalf("expected %q, got %q", LanguageEnglish, lang)
	}
	if confidence < 0.6 {
		t.Fatalf("expected confidence >= 0.6, got %0.2f", confidence)
	}
}

func TestDetectLanguageNorwegian(t *testing.T) {
	lang, confidence := DetectLanguage("Dette er en enkel test på norsk.")
	if lang != LanguageNorwegian {
		t.Fatalf("expected %q, got %q", LanguageNorwegian, lang)
	}
	if confidence < 0.6 {
		t.Fatalf("expected confidence >= 0.6, got %0.2f", confidence)
	}
}

func TestDetectLanguageUnknown(t *testing.T) {
	lang, confidence := DetectLanguage("QWERTY ZXCVB")
	if lang != "unknown" {
		t.Fatalf("expected unknown language, got %q", lang)
	}
	if confidence != 0 {
		t.Fatalf("expected 0 confidence, got %0.2f", confidence)
	}
}

func TestFilterItemsByLanguage(t *testing.T) {
	items := []connectors.Item{
		{Title: "This is a test of the system."},
		{Title: "Dette er en test på norsk."},
		{Title: "ZXCVB QWERTY"},
	}
	filtered := FilterItemsByLanguage(items, []string{"en", "no"}, 0.6)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 items, got %d", len(filtered))
	}
	if filtered[0].Title == items[2].Title || filtered[1].Title == items[2].Title {
		t.Fatalf("expected unknown language item to be filtered out")
	}
}
