package storage

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSeenItemUpsert(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	first := time.Unix(1700000000, 0).UTC()
	initial := SeenItemRecord{
		CanonicalURL:   "https://example.com/one",
		Title:          "First",
		FirstSeenAt:    first,
		LastSeenAt:     first,
		SourcePlatform: "reddit",
		SourceType:     "subreddits",
		SourceID:       "golang",
	}

	if err := UpsertSeenItems(db, []SeenItemRecord{initial}); err != nil {
		t.Fatalf("upsert initial: %v", err)
	}

	stored, ok, err := GetSeenItem(db, initial.CanonicalURL)
	if err != nil {
		t.Fatalf("get seen item: %v", err)
	}
	if !ok {
		t.Fatalf("expected seen item")
	}
	if stored.FirstSeenAt.Unix() != first.Unix() {
		t.Fatalf("expected first_seen_at %v, got %v", first, stored.FirstSeenAt)
	}
	if stored.LastSeenAt.Unix() != first.Unix() {
		t.Fatalf("expected last_seen_at %v, got %v", first, stored.LastSeenAt)
	}
	if stored.Title != "First" {
		t.Fatalf("expected title to persist")
	}
	if stored.SourcePlatform != "reddit" {
		t.Fatalf("expected source platform to persist")
	}

	second := first.Add(2 * time.Hour)
	update := SeenItemRecord{
		CanonicalURL:   "https://example.com/one",
		LastSeenAt:     second,
		SourcePlatform: "hn",
		SourceType:     "top",
	}
	if err := UpsertSeenItems(db, []SeenItemRecord{update}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	updated, ok, err := GetSeenItem(db, initial.CanonicalURL)
	if err != nil {
		t.Fatalf("get updated seen item: %v", err)
	}
	if !ok {
		t.Fatalf("expected updated seen item")
	}
	if updated.FirstSeenAt.Unix() != first.Unix() {
		t.Fatalf("expected first_seen_at to remain")
	}
	if updated.LastSeenAt.Unix() != second.Unix() {
		t.Fatalf("expected last_seen_at to update")
	}
	if updated.Title != "First" {
		t.Fatalf("expected title to remain when empty update")
	}
	if updated.SourcePlatform != "hn" {
		t.Fatalf("expected source platform to update")
	}
	if updated.SourceType != "top" {
		t.Fatalf("expected source type to update")
	}
	if updated.SourceID != "golang" {
		t.Fatalf("expected source identifier to remain")
	}
}

func TestPruneSeenItemsBefore(t *testing.T) {
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

	if err := UpsertSeenItems(db, []SeenItemRecord{
		{
			CanonicalURL: "https://example.com/old",
			Title:        "Old",
			FirstSeenAt:  first,
			LastSeenAt:   first,
		},
		{
			CanonicalURL: "https://example.com/new",
			Title:        "New",
			FirstSeenAt:  second,
			LastSeenAt:   second,
		},
	}); err != nil {
		t.Fatalf("upsert seen items: %v", err)
	}

	deleted, err := PruneSeenItemsBefore(db, first.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("prune seen items: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 seen item deleted, got %d", deleted)
	}

	_, ok, err := GetSeenItem(db, "https://example.com/old")
	if err != nil {
		t.Fatalf("get old seen item: %v", err)
	}
	if ok {
		t.Fatalf("expected old seen item to be pruned")
	}

	_, ok, err = GetSeenItem(db, "https://example.com/new")
	if err != nil {
		t.Fatalf("get new seen item: %v", err)
	}
	if !ok {
		t.Fatalf("expected new seen item to remain")
	}
}
