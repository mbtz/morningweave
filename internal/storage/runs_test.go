package storage

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestRunCRUD(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	startedAt := time.Unix(1700000000, 0).UTC()
	record := RunRecord{
		StartedAt:    startedAt,
		Status:       "success",
		ScopeType:    "tag",
		ScopeName:    "ai",
		ItemsFetched: 12,
		ItemsRanked:  8,
		ItemsSent:    5,
		EmailSent:    true,
		PlatformCounts: map[string]int{
			"reddit": 3,
			"hn":     2,
		},
	}

	created, err := CreateRun(db, record)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected run id")
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("expected created_at timestamp")
	}
	if created.StartedAt.Unix() != startedAt.Unix() {
		t.Fatalf("expected started_at %v, got %v", startedAt, created.StartedAt)
	}

	latest, ok, err := GetLastRun(db)
	if err != nil {
		t.Fatalf("get last run: %v", err)
	}
	if !ok {
		t.Fatalf("expected run record")
	}
	if latest.ID != created.ID {
		t.Fatalf("expected id %d, got %d", created.ID, latest.ID)
	}
	if latest.PlatformCounts["reddit"] != 3 {
		t.Fatalf("expected platform count")
	}

	latest.Status = "empty"
	latest.ItemsSent = 0
	latest.EmailSent = false
	latest.Error = "no items"
	latest.FinishedAt = latest.StartedAt.Add(5 * time.Minute)

	if err := UpdateRun(db, latest); err != nil {
		t.Fatalf("update run: %v", err)
	}

	updated, ok, err := GetLastRun(db)
	if err != nil {
		t.Fatalf("get last run after update: %v", err)
	}
	if !ok {
		t.Fatalf("expected updated run")
	}
	if updated.Status != "empty" {
		t.Fatalf("expected status to update")
	}
	if updated.EmailSent {
		t.Fatalf("expected email_sent false")
	}
	if updated.Error != "no items" {
		t.Fatalf("expected error to update")
	}
	if updated.FinishedAt.IsZero() {
		t.Fatalf("expected finished_at set")
	}

	records, err := ListRuns(db, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 run, got %d", len(records))
	}

	second := RunRecord{
		StartedAt: startedAt.Add(2 * time.Hour),
		Status:    "success",
	}
	_, err = CreateRun(db, second)
	if err != nil {
		t.Fatalf("create second run: %v", err)
	}

	filtered, err := ListRunsSince(db, startedAt.Add(30*time.Minute), 10)
	if err != nil {
		t.Fatalf("list runs since: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered run, got %d", len(filtered))
	}
	if filtered[0].StartedAt.Unix() != second.StartedAt.Unix() {
		t.Fatalf("expected filtered run to match second run")
	}
}

func TestGetLastRunForScope(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	base := time.Unix(1700000000, 0).UTC()
	if _, err := CreateRun(db, RunRecord{StartedAt: base, Status: "success", ScopeType: "global"}); err != nil {
		t.Fatalf("create global run: %v", err)
	}
	if _, err := CreateRun(db, RunRecord{StartedAt: base.Add(2 * time.Hour), Status: "success", ScopeType: "tag", ScopeName: "ai"}); err != nil {
		t.Fatalf("create tag run: %v", err)
	}
	if _, err := CreateRun(db, RunRecord{StartedAt: base.Add(3 * time.Hour), Status: "success", ScopeType: "tag", ScopeName: "work"}); err != nil {
		t.Fatalf("create second tag run: %v", err)
	}

	latest, ok, err := GetLastRunForScope(db, "tag", "ai")
	if err != nil {
		t.Fatalf("get last run for scope: %v", err)
	}
	if !ok {
		t.Fatalf("expected tag run")
	}
	if latest.ScopeName != "ai" {
		t.Fatalf("expected tag ai, got %s", latest.ScopeName)
	}
	if latest.StartedAt.Unix() != base.Add(2*time.Hour).Unix() {
		t.Fatalf("expected latest ai run")
	}

	_, ok, err = GetLastRunForScope(db, "tag", "missing")
	if err != nil {
		t.Fatalf("get missing scope: %v", err)
	}
	if ok {
		t.Fatalf("expected missing scope to be false")
	}

	global, ok, err := GetLastRunForScope(db, "global", "")
	if err != nil {
		t.Fatalf("get global scope: %v", err)
	}
	if !ok {
		t.Fatalf("expected global scope run")
	}
	if global.ScopeType != "global" {
		t.Fatalf("expected global scope type")
	}
}

func TestPruneRunsBefore(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	first := time.Unix(1700000000, 0).UTC()
	second := first.Add(48 * time.Hour)
	if _, err := CreateRun(db, RunRecord{StartedAt: first, Status: "success"}); err != nil {
		t.Fatalf("create first run: %v", err)
	}
	if _, err := CreateRun(db, RunRecord{StartedAt: second, Status: "success"}); err != nil {
		t.Fatalf("create second run: %v", err)
	}

	deleted, err := PruneRunsBefore(db, first.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("prune runs: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 run deleted, got %d", deleted)
	}

	records, err := ListRuns(db, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 run remaining, got %d", len(records))
	}
	if records[0].StartedAt.Unix() != second.Unix() {
		t.Fatalf("expected remaining run to be second")
	}
}
