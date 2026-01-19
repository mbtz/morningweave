package runner

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"morningweave/internal/storage"

	_ "modernc.org/sqlite"
)

func TestAdaptiveWeightsThresholdAndApply(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := storage.ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	scope := RunScope{Type: "tag", Name: "AI", Weight: 2.0}
	now := time.Unix(1700000000, 0).UTC()

	for i := 0; i < 9; i++ {
		if err := updateAdaptiveWeights(db, scope, "success", 1, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("update adaptive weights: %v", err)
		}
	}

	record, ok, err := storage.GetTagWeight(db, "tag:ai")
	if err != nil {
		t.Fatalf("get tag weight: %v", err)
	}
	if !ok {
		t.Fatalf("expected tag weight record")
	}
	if record.Runs != 9 || record.Hits != 9 {
		t.Fatalf("expected runs/hits 9/9, got %d/%d", record.Runs, record.Hits)
	}
	if record.Weight != 1.0 {
		t.Fatalf("expected weight 1.0 before threshold, got %v", record.Weight)
	}

	if err := updateAdaptiveWeights(db, scope, "empty", 0, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("update adaptive weights threshold: %v", err)
	}

	record, ok, err = storage.GetTagWeight(db, "tag:ai")
	if err != nil {
		t.Fatalf("get tag weight after threshold: %v", err)
	}
	if !ok {
		t.Fatalf("expected tag weight record after threshold")
	}
	if record.Runs != 10 || record.Hits != 9 {
		t.Fatalf("expected runs/hits 10/9, got %d/%d", record.Runs, record.Hits)
	}
	expected := adaptiveMinMultiplier + (adaptiveMaxMultiplier-adaptiveMinMultiplier)*0.9
	if math.Abs(record.Weight-expected) > 0.0001 {
		t.Fatalf("expected weight %.2f, got %.4f", expected, record.Weight)
	}

	adjusted, err := applyAdaptiveWeight(db, scope)
	if err != nil {
		t.Fatalf("apply adaptive weight: %v", err)
	}
	if math.Abs(adjusted.Weight-(scope.Weight*expected)) > 0.0001 {
		t.Fatalf("expected adjusted weight %.4f, got %.4f", scope.Weight*expected, adjusted.Weight)
	}
}
