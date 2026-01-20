package runner

import (
	"strings"
	"testing"
)

func TestAuthRequirementHint(t *testing.T) {
	hint := AuthRequirementHint("x")
	if hint == "" {
		t.Fatal("expected hint for x")
	}
	if !strings.Contains(hint, "scopes:") {
		t.Fatalf("expected scopes in hint, got %q", hint)
	}
	if !strings.Contains(hint, "tweet.read") || !strings.Contains(hint, "users.read") {
		t.Fatalf("expected x scopes in hint, got %q", hint)
	}
	if !strings.Contains(hint, "notes:") {
		t.Fatalf("expected notes in hint, got %q", hint)
	}

	if got := AuthRequirementHint("hn"); got != "" {
		t.Fatalf("expected empty hint for hn, got %q", got)
	}
	if got := AuthRequirementHint("unknown"); got != "" {
		t.Fatalf("expected empty hint for unknown platform, got %q", got)
	}
}

func TestAppendAuthRequirementHint(t *testing.T) {
	warnings := []string{"x: credentials_ref is required"}
	warnings = AppendAuthRequirementHint(warnings, "x")
	if len(warnings) != 2 {
		t.Fatalf("expected hint appended, got %d warnings", len(warnings))
	}
	warnings = AppendAuthRequirementHint(warnings, "x")
	if len(warnings) != 2 {
		t.Fatalf("expected no duplicate hints, got %d warnings", len(warnings))
	}
}
