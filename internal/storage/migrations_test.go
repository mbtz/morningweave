package storage

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyMigrationsCreatesTables(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := ApplyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table names: %v", err)
	}

	expected := []string{
		"schema_migrations",
		"runs",
		"seen_items",
		"dedupe_map",
		"tag_weights",
	}
	for _, name := range expected {
		if !found[name] {
			t.Fatalf("expected table %s", name)
		}
	}
}
