package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SeenItemRecord stores the dedupe tracking metadata for a canonical URL.
type SeenItemRecord struct {
	CanonicalURL   string
	Title          string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	SourcePlatform string
	SourceType     string
	SourceID       string
}

// UpsertSeenItems inserts or updates seen item records in a single transaction.
func UpsertSeenItems(db *sql.DB, records []SeenItemRecord) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if len(records) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin seen items upsert: %w", err)
	}

	for _, record := range records {
		if err := upsertSeenItem(tx, record); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seen items upsert: %w", err)
	}
	return nil
}

// GetSeenItem returns the stored seen-item metadata for a canonical URL.
func GetSeenItem(db *sql.DB, canonicalURL string) (SeenItemRecord, bool, error) {
	if db == nil {
		return SeenItemRecord{}, false, errors.New("db is nil")
	}
	canonical := strings.TrimSpace(canonicalURL)
	if canonical == "" {
		return SeenItemRecord{}, false, errors.New("canonical url is required")
	}

	row := db.QueryRow(`SELECT
			canonical_url,
			title,
			first_seen_at,
			last_seen_at,
			source_platform,
			source_type,
			source_identifier
		FROM seen_items
		WHERE canonical_url = ?`, canonical)

	var record SeenItemRecord
	var firstSeen int64
	var lastSeen int64
	if err := row.Scan(
		&record.CanonicalURL,
		&record.Title,
		&firstSeen,
		&lastSeen,
		&record.SourcePlatform,
		&record.SourceType,
		&record.SourceID,
	); err != nil {
		if err == sql.ErrNoRows {
			return SeenItemRecord{}, false, nil
		}
		return SeenItemRecord{}, false, fmt.Errorf("scan seen item: %w", err)
	}

	record.FirstSeenAt = time.Unix(firstSeen, 0)
	record.LastSeenAt = time.Unix(lastSeen, 0)

	return record, true, nil
}

type seenItemExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertSeenItem(exec seenItemExecutor, record SeenItemRecord) error {
	canonical := strings.TrimSpace(record.CanonicalURL)
	if canonical == "" {
		return errors.New("canonical url is required")
	}

	firstSeen := record.FirstSeenAt
	lastSeen := record.LastSeenAt
	if firstSeen.IsZero() {
		if lastSeen.IsZero() {
			firstSeen = time.Now()
		} else {
			firstSeen = lastSeen
		}
	}
	if lastSeen.IsZero() {
		lastSeen = firstSeen
	}

	_, err := exec.Exec(`INSERT INTO seen_items (
			canonical_url,
			title,
			first_seen_at,
			last_seen_at,
			source_platform,
			source_type,
			source_identifier
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_url) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			title = CASE
				WHEN excluded.title != '' THEN excluded.title
				ELSE seen_items.title
			END,
			source_platform = CASE
				WHEN excluded.source_platform != '' THEN excluded.source_platform
				ELSE seen_items.source_platform
			END,
			source_type = CASE
				WHEN excluded.source_type != '' THEN excluded.source_type
				ELSE seen_items.source_type
			END,
			source_identifier = CASE
				WHEN excluded.source_identifier != '' THEN excluded.source_identifier
				ELSE seen_items.source_identifier
			END`,
		canonical,
		record.Title,
		firstSeen.Unix(),
		lastSeen.Unix(),
		record.SourcePlatform,
		record.SourceType,
		record.SourceID,
	)
	if err != nil {
		return fmt.Errorf("upsert seen item: %w", err)
	}
	return nil
}

// PruneSeenItemsBefore deletes seen items with last_seen_at before the cutoff.
func PruneSeenItemsBefore(db *sql.DB, before time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("db is nil")
	}
	if before.IsZero() {
		return 0, errors.New("cutoff time is required")
	}

	result, err := db.Exec(`DELETE FROM seen_items WHERE last_seen_at < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("prune seen items: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune seen items: %w", err)
	}
	return deleted, nil
}
