package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RunRecord captures a single run outcome.
type RunRecord struct {
	ID             int64
	StartedAt      time.Time
	FinishedAt     time.Time
	Status         string
	ScopeType      string
	ScopeName      string
	ItemsFetched   int
	ItemsRanked    int
	ItemsSent      int
	EmailSent      bool
	Error          string
	PlatformCounts map[string]int
	CreatedAt      time.Time
}

// CreateRun inserts a new run record and returns the stored record with ID.
func CreateRun(db *sql.DB, record RunRecord) (RunRecord, error) {
	if db == nil {
		return RunRecord{}, errors.New("db is nil")
	}
	if record.Status == "" {
		return RunRecord{}, errors.New("status is required")
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now()
	}

	countsValue, err := encodePlatformCounts(record.PlatformCounts)
	if err != nil {
		return RunRecord{}, err
	}

	finishedValue := toNullUnix(record.FinishedAt)

	result, err := db.Exec(
		`INSERT INTO runs (
			started_at,
			finished_at,
			status,
			scope_type,
			scope_name,
			items_fetched,
			items_ranked,
			items_sent,
			email_sent,
			error,
			platform_counts
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.StartedAt.Unix(),
		optionalNullInt(finishedValue),
		record.Status,
		optionalString(record.ScopeType),
		optionalString(record.ScopeName),
		record.ItemsFetched,
		record.ItemsRanked,
		record.ItemsSent,
		boolToInt(record.EmailSent),
		optionalString(record.Error),
		optionalNullString(countsValue),
	)
	if err != nil {
		return RunRecord{}, fmt.Errorf("insert run: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return RunRecord{}, fmt.Errorf("fetch run id: %w", err)
	}
	record.ID = id

	createdAt, err := fetchRunCreatedAt(db, id)
	if err != nil {
		return RunRecord{}, err
	}
	record.CreatedAt = createdAt

	return record, nil
}

// UpdateRun updates an existing run record by ID.
func UpdateRun(db *sql.DB, record RunRecord) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if record.ID == 0 {
		return errors.New("run id is required")
	}
	if record.Status == "" {
		return errors.New("status is required")
	}
	if record.StartedAt.IsZero() {
		return errors.New("started_at is required")
	}

	countsValue, err := encodePlatformCounts(record.PlatformCounts)
	if err != nil {
		return err
	}

	finishedValue := toNullUnix(record.FinishedAt)

	_, err = db.Exec(
		`UPDATE runs SET
			started_at = ?,
			finished_at = ?,
			status = ?,
			scope_type = ?,
			scope_name = ?,
			items_fetched = ?,
			items_ranked = ?,
			items_sent = ?,
			email_sent = ?,
			error = ?,
			platform_counts = ?
		WHERE id = ?`,
		record.StartedAt.Unix(),
		optionalNullInt(finishedValue),
		record.Status,
		optionalString(record.ScopeType),
		optionalString(record.ScopeName),
		record.ItemsFetched,
		record.ItemsRanked,
		record.ItemsSent,
		boolToInt(record.EmailSent),
		optionalString(record.Error),
		optionalNullString(countsValue),
		record.ID,
	)
	if err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	return nil
}

// GetLastRun returns the most recent run by started_at.
func GetLastRun(db *sql.DB) (RunRecord, bool, error) {
	if db == nil {
		return RunRecord{}, false, errors.New("db is nil")
	}

	row := db.QueryRow(`SELECT
			id,
			started_at,
			finished_at,
			status,
			scope_type,
			scope_name,
			items_fetched,
			items_ranked,
			items_sent,
			email_sent,
			error,
			platform_counts,
			created_at
		FROM runs
		ORDER BY started_at DESC, id DESC
		LIMIT 1`)

	record, err := scanRun(row)
	if err == sql.ErrNoRows {
		return RunRecord{}, false, nil
	}
	if err != nil {
		return RunRecord{}, false, err
	}
	return record, true, nil
}

// GetLastRunForScope returns the most recent run for the provided scope.
func GetLastRunForScope(db *sql.DB, scopeType string, scopeName string) (RunRecord, bool, error) {
	if db == nil {
		return RunRecord{}, false, errors.New("db is nil")
	}

	clauses := []string{}
	args := []any{}
	if scopeType == "" {
		clauses = append(clauses, "scope_type IS NULL")
	} else {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeName == "" {
		clauses = append(clauses, "scope_name IS NULL")
	} else {
		clauses = append(clauses, "scope_name = ?")
		args = append(args, scopeName)
	}

	query := fmt.Sprintf(`SELECT
			id,
			started_at,
			finished_at,
			status,
			scope_type,
			scope_name,
			items_fetched,
			items_ranked,
			items_sent,
			email_sent,
			error,
			platform_counts,
			created_at
		FROM runs
		WHERE %s
		ORDER BY started_at DESC, id DESC
		LIMIT 1`, strings.Join(clauses, " AND "))

	row := db.QueryRow(query, args...)

	record, err := scanRun(row)
	if err == sql.ErrNoRows {
		return RunRecord{}, false, nil
	}
	if err != nil {
		return RunRecord{}, false, err
	}
	return record, true, nil
}

// ListRuns returns runs ordered by started_at, newest first.
func ListRuns(db *sql.DB, limit int) ([]RunRecord, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(`SELECT
			id,
			started_at,
			finished_at,
			status,
			scope_type,
			scope_name,
			items_fetched,
			items_ranked,
			items_sent,
			email_sent,
			error,
			platform_counts,
			created_at
		FROM runs
		ORDER BY started_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var records []RunRecord
	for rows.Next() {
		record, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return records, nil
}

// ListRunsSince returns runs ordered by started_at, newest first, filtered by since.
func ListRunsSince(db *sql.DB, since time.Time, limit int) ([]RunRecord, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(`SELECT
			id,
			started_at,
			finished_at,
			status,
			scope_type,
			scope_name,
			items_fetched,
			items_ranked,
			items_sent,
			email_sent,
			error,
			platform_counts,
			created_at
		FROM runs
		WHERE started_at >= ?
		ORDER BY started_at DESC, id DESC
		LIMIT ?`, since.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list runs since: %w", err)
	}
	defer rows.Close()

	var records []RunRecord
	for rows.Next() {
		record, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs since: %w", err)
	}
	return records, nil
}

// PruneRunsBefore deletes runs with started_at before the cutoff.
func PruneRunsBefore(db *sql.DB, before time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("db is nil")
	}
	if before.IsZero() {
		return 0, errors.New("cutoff time is required")
	}

	result, err := db.Exec(`DELETE FROM runs WHERE started_at < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("prune runs: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune runs: %w", err)
	}
	return deleted, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(scanner rowScanner) (RunRecord, error) {
	var record RunRecord
	var startedAt int64
	var finishedAt sql.NullInt64
	var emailSent int
	var errorText sql.NullString
	var platformCounts sql.NullString
	var createdAt int64
	var scopeType sql.NullString
	var scopeName sql.NullString

	if err := scanner.Scan(
		&record.ID,
		&startedAt,
		&finishedAt,
		&record.Status,
		&scopeType,
		&scopeName,
		&record.ItemsFetched,
		&record.ItemsRanked,
		&record.ItemsSent,
		&emailSent,
		&errorText,
		&platformCounts,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunRecord{}, sql.ErrNoRows
		}
		return RunRecord{}, fmt.Errorf("scan run: %w", err)
	}

	record.StartedAt = time.Unix(startedAt, 0)
	record.FinishedAt = fromNullUnix(finishedAt)
	record.EmailSent = emailSent != 0
	if scopeType.Valid {
		record.ScopeType = scopeType.String
	}
	if scopeName.Valid {
		record.ScopeName = scopeName.String
	}
	if errorText.Valid {
		record.Error = errorText.String
	}
	counts, err := decodePlatformCounts(platformCounts)
	if err != nil {
		return RunRecord{}, err
	}
	record.PlatformCounts = counts
	record.CreatedAt = time.Unix(createdAt, 0)

	return record, nil
}

func fetchRunCreatedAt(db *sql.DB, id int64) (time.Time, error) {
	var createdAt int64
	row := db.QueryRow("SELECT created_at FROM runs WHERE id = ?", id)
	if err := row.Scan(&createdAt); err != nil {
		return time.Time{}, fmt.Errorf("fetch created_at: %w", err)
	}
	return time.Unix(createdAt, 0), nil
}

func encodePlatformCounts(counts map[string]int) (sql.NullString, error) {
	if len(counts) == 0 {
		return sql.NullString{}, nil
	}
	payload, err := json.Marshal(counts)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("encode platform counts: %w", err)
	}
	return sql.NullString{String: string(payload), Valid: true}, nil
}

func decodePlatformCounts(value sql.NullString) (map[string]int, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	var counts map[string]int
	if err := json.Unmarshal([]byte(value.String), &counts); err != nil {
		return nil, fmt.Errorf("decode platform counts: %w", err)
	}
	return counts, nil
}

func toNullUnix(value time.Time) sql.NullInt64 {
	if value.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value.Unix(), Valid: true}
}

func fromNullUnix(value sql.NullInt64) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return time.Unix(value.Int64, 0)
}

func optionalNullInt(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func optionalNullString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
