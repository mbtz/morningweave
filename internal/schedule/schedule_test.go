package schedule

import (
	"testing"
	"time"
)

func TestParseAndNextRunSameDay(t *testing.T) {
	spec, err := Parse("0 7 * * *")
	if err != nil {
		t.Fatalf("expected parse to succeed: %v", err)
	}

	from := time.Date(2026, 1, 18, 6, 0, 0, 0, time.UTC)
	next, err := spec.Next(from)
	if err != nil {
		t.Fatalf("expected next run: %v", err)
	}

	expected := time.Date(2026, 1, 18, 7, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected.Format(time.RFC3339), next.Format(time.RFC3339))
	}
}

func TestParseAndNextRunNextDay(t *testing.T) {
	from := time.Date(2026, 1, 18, 8, 0, 0, 0, time.UTC)
	next, err := NextRun("0 7 * * *", from)
	if err != nil {
		t.Fatalf("expected parse to succeed: %v", err)
	}

	expected := time.Date(2026, 1, 19, 7, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected.Format(time.RFC3339), next.Format(time.RFC3339))
	}
}

func TestParseRejectsWrongFieldCount(t *testing.T) {
	_, err := Parse("0 7 * *")
	if err == nil {
		t.Fatal("expected error for 4-field schedule")
	}

	_, err = Parse("0 7 * * * *")
	if err == nil {
		t.Fatal("expected error for 6-field schedule")
	}
}
