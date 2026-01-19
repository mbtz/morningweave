package ranking

import (
	"testing"
	"time"

	"morningweave/internal/connectors"
)

func TestRankItemsFiltersByLanguageAndKeyword(t *testing.T) {
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	items := []connectors.Item{
		{
			Title:     "the cat is in the house",
			Text:      "the cat is running",
			Timestamp: now.Add(-2 * time.Hour),
			Source:    connectors.SourceRef{Platform: "reddit"},
		},
		{
			Title:     "jeg og er her",
			Text:      "vi er hjemme",
			Timestamp: now.Add(-3 * time.Hour),
			Source:    connectors.SourceRef{Platform: "hn"},
		},
	}

	scored := RankItems(items, RankOptions{
		Keywords:              []string{"cat"},
		AllowedLanguages:      []string{"en"},
		MinLanguageConfidence: 0.5,
		Now:                   now,
	})

	if len(scored) != 1 {
		t.Fatalf("expected 1 item after filtering, got %d", len(scored))
	}
	if scored[0].Item.Title != "the cat is in the house" {
		t.Fatalf("expected english item to remain, got %q", scored[0].Item.Title)
	}
}

func TestRankItemsUsesSourceWeight(t *testing.T) {
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	items := []connectors.Item{
		{
			Title:     "the cat is in the house",
			Text:      "the cat is running",
			Timestamp: now.Add(-1 * time.Hour),
			Source:    connectors.SourceRef{Platform: "reddit", Identifier: "high"},
		},
		{
			Title:     "the cat is in the house",
			Text:      "the cat is running",
			Timestamp: now.Add(-1 * time.Hour),
			Source:    connectors.SourceRef{Platform: "reddit", Identifier: "low"},
		},
	}

	weightFn := func(item connectors.Item) float64 {
		if item.Source.Identifier == "high" {
			return 2.0
		}
		return 0.5
	}

	scored := RankItems(items, RankOptions{
		Keywords:     []string{"cat"},
		Now:          now,
		SourceWeight: weightFn,
	})

	if len(scored) != 2 {
		t.Fatalf("expected 2 items, got %d", len(scored))
	}
	if scored[0].SourceWeight < scored[1].SourceWeight {
		t.Fatalf("expected higher source weight to rank first")
	}
	if scored[0].Score <= scored[1].Score {
		t.Fatalf("expected higher score for weighted item: %0.3f <= %0.3f", scored[0].Score, scored[1].Score)
	}
}

func TestRankItemsSortsByRecency(t *testing.T) {
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	older := connectors.Item{
		Title:     "the cat is in the house",
		Text:      "the cat is running",
		Timestamp: now.Add(-6 * time.Hour),
		Source:    connectors.SourceRef{Platform: "reddit", Identifier: "older"},
	}
	newer := connectors.Item{
		Title:     "the cat is in the house",
		Text:      "the cat is running",
		Timestamp: now.Add(-1 * time.Hour),
		Source:    connectors.SourceRef{Platform: "reddit", Identifier: "newer"},
	}

	scored := RankItems([]connectors.Item{older, newer}, RankOptions{
		Keywords: []string{"cat"},
		Now:      now,
	})

	if len(scored) != 2 {
		t.Fatalf("expected 2 items, got %d", len(scored))
	}
	if scored[0].Item.Source.Identifier != "newer" {
		t.Fatalf("expected newer item to rank first")
	}
}
