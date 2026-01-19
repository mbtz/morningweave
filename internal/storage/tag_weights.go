package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TagWeightRecord captures adaptive weighting stats per tag or category.
type TagWeightRecord struct {
	TagName   string
	Weight    float64
	Runs      int
	Hits      int
	UpdatedAt time.Time
}

// UpsertTagWeights inserts or updates tag weight records in a single transaction.
func UpsertTagWeights(db *sql.DB, records []TagWeightRecord) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if len(records) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tag_weights upsert: %w", err)
	}

	for _, record := range records {
		if err := upsertTagWeight(tx, record); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tag_weights upsert: %w", err)
	}
	return nil
}

// GetTagWeight returns the stored adaptive weight for a tag/category.
func GetTagWeight(db *sql.DB, tagName string) (TagWeightRecord, bool, error) {
	if db == nil {
		return TagWeightRecord{}, false, errors.New("db is nil")
	}
	name := strings.TrimSpace(tagName)
	if name == "" {
		return TagWeightRecord{}, false, errors.New("tag name is required")
	}

	row := db.QueryRow(`SELECT
            tag_name,
            weight,
            runs,
            hits,
            updated_at
        FROM tag_weights
        WHERE tag_name = ?`, name)

	record, err := scanTagWeight(row)
	if err == sql.ErrNoRows {
		return TagWeightRecord{}, false, nil
	}
	if err != nil {
		return TagWeightRecord{}, false, err
	}
	return record, true, nil
}

// ListTagWeights returns all stored adaptive weights ordered by tag name.
func ListTagWeights(db *sql.DB) ([]TagWeightRecord, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}

	rows, err := db.Query(`SELECT
            tag_name,
            weight,
            runs,
            hits,
            updated_at
        FROM tag_weights
        ORDER BY tag_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tag_weights: %w", err)
	}
	defer rows.Close()

	var records []TagWeightRecord
	for rows.Next() {
		record, err := scanTagWeight(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag_weights: %w", err)
	}
	return records, nil
}

type tagWeightExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertTagWeight(exec tagWeightExecutor, record TagWeightRecord) error {
	name := strings.TrimSpace(record.TagName)
	if name == "" {
		return errors.New("tag name is required")
	}
	if record.Runs < 0 {
		return errors.New("runs must be >= 0")
	}
	if record.Hits < 0 {
		return errors.New("hits must be >= 0")
	}

	updated := record.UpdatedAt
	if updated.IsZero() {
		updated = time.Now()
	}

	_, err := exec.Exec(`INSERT INTO tag_weights (
            tag_name,
            weight,
            runs,
            hits,
            updated_at
        ) VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(tag_name) DO UPDATE SET
            weight = excluded.weight,
            runs = excluded.runs,
            hits = excluded.hits,
            updated_at = excluded.updated_at`,
		name,
		record.Weight,
		record.Runs,
		record.Hits,
		updated.Unix(),
	)
	if err != nil {
		return fmt.Errorf("upsert tag weight: %w", err)
	}
	return nil
}

type tagWeightScanner interface {
	Scan(dest ...any) error
}

func scanTagWeight(scanner tagWeightScanner) (TagWeightRecord, error) {
	var record TagWeightRecord
	var updatedAt int64
	if err := scanner.Scan(
		&record.TagName,
		&record.Weight,
		&record.Runs,
		&record.Hits,
		&updatedAt,
	); err != nil {
		return TagWeightRecord{}, err
	}
	record.UpdatedAt = time.Unix(updatedAt, 0)
	return record, nil
}
