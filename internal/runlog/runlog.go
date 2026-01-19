package runlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry captures a structured run log entry.
type Entry struct {
	ID             int64          `json:"id"`
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     time.Time      `json:"finished_at,omitempty"`
	Status         string         `json:"status"`
	ScopeType      string         `json:"scope_type,omitempty"`
	ScopeName      string         `json:"scope_name,omitempty"`
	ItemsFetched   int            `json:"items_fetched"`
	ItemsRanked    int            `json:"items_ranked"`
	ItemsSent      int            `json:"items_sent"`
	EmailSent      bool           `json:"email_sent"`
	PlatformCounts map[string]int `json:"platform_counts,omitempty"`
	Error          string         `json:"error,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
	DurationSec    float64        `json:"duration_seconds,omitempty"`
}

// Write persists a run log entry to the default log path derived from storagePath.
func Write(storagePath string, entry Entry, secrets map[string]string) error {
	if strings.TrimSpace(storagePath) == "" {
		return errors.New("storage path is required")
	}

	sanitized := redactEntry(entry, secrets)
	if sanitized.StartedAt.IsZero() {
		sanitized.StartedAt = time.Now()
	}
	if !sanitized.FinishedAt.IsZero() {
		duration := sanitized.FinishedAt.Sub(sanitized.StartedAt).Seconds()
		if duration >= 0 {
			sanitized.DurationSec = duration
		}
	}

	path := pathFor(storagePath, sanitized)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	data, err := json.Marshal(sanitized)
	if err != nil {
		return fmt.Errorf("marshal log entry: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write log entry: %w", err)
	}
	return nil
}

// Prune deletes log files older than cutoff.
func Prune(storagePath string, cutoff time.Time) (int, error) {
	if strings.TrimSpace(storagePath) == "" {
		return 0, errors.New("storage path is required")
	}
	if cutoff.IsZero() {
		return 0, nil
	}
	logDir := baseDir(storagePath)
	info, err := os.Stat(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat log dir: %w", err)
	}
	if !info.IsDir() {
		return 0, nil
	}

	removed := 0
	err = filepath.WalkDir(logDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	if err != nil {
		return removed, fmt.Errorf("prune logs: %w", err)
	}
	return removed, nil
}

func pathFor(storagePath string, entry Entry) string {
	date := entry.StartedAt.Format("2006-01-02")
	filename := fmt.Sprintf("run-%d.json", entry.ID)
	return filepath.Join(baseDir(storagePath), date, filename)
}

func baseDir(storagePath string) string {
	base := filepath.Dir(storagePath)
	return filepath.Join(base, "logs")
}

func redactEntry(entry Entry, secrets map[string]string) Entry {
	if len(secrets) == 0 {
		return entry
	}
	entry.Error = redactString(entry.Error, secrets)
	if len(entry.Warnings) > 0 {
		warnings := make([]string, 0, len(entry.Warnings))
		for _, warning := range entry.Warnings {
			warnings = append(warnings, redactString(warning, secrets))
		}
		entry.Warnings = warnings
	}
	return entry
}

func redactString(value string, secrets map[string]string) string {
	if value == "" || len(secrets) == 0 {
		return value
	}
	redacted := value
	for _, secret := range secrets {
		trimmed := strings.TrimSpace(secret)
		if trimmed == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, trimmed, "[REDACTED]")
	}
	return redacted
}
