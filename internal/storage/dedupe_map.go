package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DedupeMapRecord stores canonical URL + normalized title metadata.
type DedupeMapRecord struct {
	CanonicalURL    string
	Title           string
	NormalizedTitle string
	LastSeenAt      time.Time
}

// UpsertDedupeMap inserts or updates dedupe map records in a single transaction.
func UpsertDedupeMap(db *sql.DB, records []DedupeMapRecord) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if len(records) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin dedupe map upsert: %w", err)
	}

	for _, record := range records {
		if err := upsertDedupeMap(tx, record); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dedupe map upsert: %w", err)
	}
	return nil
}

// GetDedupeMap returns the stored dedupe map metadata for a canonical URL.
func GetDedupeMap(db *sql.DB, canonicalURL string) (DedupeMapRecord, bool, error) {
	if db == nil {
		return DedupeMapRecord{}, false, errors.New("db is nil")
	}
	canonical := strings.TrimSpace(canonicalURL)
	if canonical == "" {
		return DedupeMapRecord{}, false, errors.New("canonical url is required")
	}

	row := db.QueryRow(`SELECT
			canonical_url,
			title,
			normalized_title,
			last_seen_at
		FROM dedupe_map
		WHERE canonical_url = ?`, canonical)

	var record DedupeMapRecord
	var lastSeen int64
	if err := row.Scan(
		&record.CanonicalURL,
		&record.Title,
		&record.NormalizedTitle,
		&lastSeen,
	); err != nil {
		if err == sql.ErrNoRows {
			return DedupeMapRecord{}, false, nil
		}
		return DedupeMapRecord{}, false, fmt.Errorf("scan dedupe map: %w", err)
	}
	record.LastSeenAt = time.Unix(lastSeen, 0)

	return record, true, nil
}

// PruneDedupeMapBefore deletes dedupe map records with last_seen_at before the cutoff.
func PruneDedupeMapBefore(db *sql.DB, before time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("db is nil")
	}
	if before.IsZero() {
		return 0, errors.New("cutoff time is required")
	}

	result, err := db.Exec(`DELETE FROM dedupe_map WHERE last_seen_at < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("prune dedupe map: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune dedupe map: %w", err)
	}
	return deleted, nil
}

type dedupeMapExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertDedupeMap(exec dedupeMapExecutor, record DedupeMapRecord) error {
	canonical := strings.TrimSpace(record.CanonicalURL)
	if canonical == "" {
		return errors.New("canonical url is required")
	}
	normalized := strings.TrimSpace(record.NormalizedTitle)
	if normalized == "" {
		return errors.New("normalized title is required")
	}

	lastSeen := record.LastSeenAt
	if lastSeen.IsZero() {
		lastSeen = time.Now()
	}

	_, err := exec.Exec(`INSERT INTO dedupe_map (
			canonical_url,
			title,
			normalized_title,
			last_seen_at
		) VALUES (?, ?, ?, ?)
		ON CONFLICT(canonical_url) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			title = CASE
				WHEN excluded.title != '' THEN excluded.title
				ELSE dedupe_map.title
			END,
			normalized_title = CASE
				WHEN excluded.normalized_title != '' THEN excluded.normalized_title
				ELSE dedupe_map.normalized_title
			END`,
		canonical,
		record.Title,
		normalized,
		lastSeen.Unix(),
	)
	if err != nil {
		return fmt.Errorf("upsert dedupe map: %w", err)
	}
	return nil
}
