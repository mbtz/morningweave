package secrets

import (
	"context"
	"fmt"
	"strings"
)

func opRead(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", ErrNotFound
	}
	output, err := runCommand(context.Background(), "op", "read", trimmed)
	if err != nil {
		if isExecNotFound(err) {
			return "", fmt.Errorf("%w: 1password", ErrProviderUnavailable)
		}
		if isOpNotFound(output) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("1password read: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", ErrNotFound
	}
	return value, nil
}

func isOpNotFound(output []byte) bool {
	if len(output) == 0 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(string(output)))
	switch {
	case strings.Contains(lower, "not found"):
		return true
	case strings.Contains(lower, "no item"):
		return true
	case strings.Contains(lower, "does not exist"):
		return true
	case strings.Contains(lower, "could not be found"):
		return true
	default:
		return false
	}
}
