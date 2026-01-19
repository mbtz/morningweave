package storage

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestTagWeightsCRUD(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	record := TagWeightRecord{
		TagName:   "ai",
		Weight:    1.2,
		Runs:      10,
		Hits:      4,
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
	}

	if err := UpsertTagWeights(db, []TagWeightRecord{record}); err != nil {
		t.Fatalf("upsert tag weights: %v", err)
	}

	loaded, ok, err := GetTagWeight(db, "ai")
	if err != nil {
		t.Fatalf("get tag weight: %v", err)
	}
	if !ok {
		t.Fatalf("expected tag weight to exist")
	}
	if loaded.TagName != "ai" {
		t.Fatalf("expected tag name ai, got %s", loaded.TagName)
	}
	if loaded.Weight != 1.2 {
		t.Fatalf("expected weight 1.2, got %v", loaded.Weight)
	}
	if loaded.Runs != 10 || loaded.Hits != 4 {
		t.Fatalf("unexpected runs/hits: %d/%d", loaded.Runs, loaded.Hits)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at to be set")
	}

	updated := TagWeightRecord{
		TagName: "ai",
		Weight:  1.05,
		Runs:    12,
		Hits:    5,
	}

	if err := UpsertTagWeights(db, []TagWeightRecord{updated}); err != nil {
		t.Fatalf("upsert updated tag weight: %v", err)
	}

	loaded, ok, err = GetTagWeight(db, "ai")
	if err != nil {
		t.Fatalf("get updated tag weight: %v", err)
	}
	if !ok {
		t.Fatalf("expected updated tag weight to exist")
	}
	if loaded.Weight != 1.05 {
		t.Fatalf("expected weight 1.05, got %v", loaded.Weight)
	}
	if loaded.Runs != 12 || loaded.Hits != 5 {
		t.Fatalf("unexpected updated runs/hits: %d/%d", loaded.Runs, loaded.Hits)
	}

	records, err := ListTagWeights(db)
	if err != nil {
		t.Fatalf("list tag weights: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 tag weight record, got %d", len(records))
	}
}
