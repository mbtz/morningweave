package storage

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestDedupeMapUpsert(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	first := time.Unix(1700000000, 0).UTC()
	initial := DedupeMapRecord{
		CanonicalURL:    "https://example.com/one",
		Title:           "First",
		NormalizedTitle: "first",
		LastSeenAt:      first,
	}

	if err := UpsertDedupeMap(db, []DedupeMapRecord{initial}); err != nil {
		t.Fatalf("upsert initial: %v", err)
	}

	stored, ok, err := GetDedupeMap(db, initial.CanonicalURL)
	if err != nil {
		t.Fatalf("get dedupe map: %v", err)
	}
	if !ok {
		t.Fatalf("expected dedupe map")
	}
	if stored.LastSeenAt.Unix() != first.Unix() {
		t.Fatalf("expected last_seen_at %v, got %v", first, stored.LastSeenAt)
	}
	if stored.Title != "First" {
		t.Fatalf("expected title to persist")
	}
	if stored.NormalizedTitle != "first" {
		t.Fatalf("expected normalized title to persist")
	}

	second := first.Add(2 * time.Hour)
	update := DedupeMapRecord{
		CanonicalURL:    "https://example.com/one",
		Title:           "",
		NormalizedTitle: "first",
		LastSeenAt:      second,
	}
	if err := UpsertDedupeMap(db, []DedupeMapRecord{update}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	updated, ok, err := GetDedupeMap(db, initial.CanonicalURL)
	if err != nil {
		t.Fatalf("get updated dedupe map: %v", err)
	}
	if !ok {
		t.Fatalf("expected updated dedupe map")
	}
	if updated.LastSeenAt.Unix() != second.Unix() {
		t.Fatalf("expected last_seen_at to update")
	}
	if updated.Title != "First" {
		t.Fatalf("expected title to remain when empty update")
	}
	if updated.NormalizedTitle != "first" {
		t.Fatalf("expected normalized title to remain")
	}
}

func TestPruneDedupeMapBefore(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	first := time.Unix(1700000000, 0).UTC()
	second := first.Add(72 * time.Hour)

	if err := UpsertDedupeMap(db, []DedupeMapRecord{
		{
			CanonicalURL:    "https://example.com/old",
			Title:           "Old",
			NormalizedTitle: "old",
			LastSeenAt:      first,
		},
		{
			CanonicalURL:    "https://example.com/new",
			Title:           "New",
			NormalizedTitle: "new",
			LastSeenAt:      second,
		},
	}); err != nil {
		t.Fatalf("upsert dedupe map: %v", err)
	}

	deleted, err := PruneDedupeMapBefore(db, first.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("prune dedupe map: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 dedupe map entry deleted, got %d", deleted)
	}

	_, ok, err := GetDedupeMap(db, "https://example.com/old")
	if err != nil {
		t.Fatalf("get old dedupe map: %v", err)
	}
	if ok {
		t.Fatalf("expected old dedupe map entry to be pruned")
	}

	_, ok, err = GetDedupeMap(db, "https://example.com/new")
	if err != nil {
		t.Fatalf("get new dedupe map: %v", err)
	}
	if !ok {
		t.Fatalf("expected new dedupe map entry to remain")
	}
}
