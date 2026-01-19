package secrets

import (
	"context"
	"fmt"
	"strings"
)

const defaultKeychainService = "morningweave"

func keychainRead(key string) (string, error) {
	service, account, ok := parseKeychainRef(key)
	if !ok {
		return "", ErrNotFound
	}
	output, err := runCommand(context.Background(), "security", "find-generic-password", "-a", account, "-s", service, "-w")
	if err != nil {
		if isExecNotFound(err) {
			return "", fmt.Errorf("%w: keychain", ErrProviderUnavailable)
		}
		if isKeychainNotFound(output) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain read: %w", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", ErrNotFound
	}
	return value, nil
}

func keychainWrite(key string, value string) error {
	service, account, ok := parseKeychainRef(key)
	if !ok {
		return ErrNotFound
	}
	_, err := runCommand(context.Background(), "security", "add-generic-password", "-a", account, "-s", service, "-w", value, "-U")
	if err != nil {
		if isExecNotFound(err) {
			return fmt.Errorf("%w: keychain", ErrProviderUnavailable)
		}
		return fmt.Errorf("keychain write: %w", err)
	}
	return nil
}

func keychainDelete(key string) error {
	service, account, ok := parseKeychainRef(key)
	if !ok {
		return ErrNotFound
	}
	output, err := runCommand(context.Background(), "security", "delete-generic-password", "-a", account, "-s", service)
	if err != nil {
		if isExecNotFound(err) {
			return fmt.Errorf("%w: keychain", ErrProviderUnavailable)
		}
		if isKeychainNotFound(output) {
			return ErrNotFound
		}
		return fmt.Errorf("keychain delete: %w", err)
	}
	return nil
}

func parseKeychainRef(key string) (string, string, bool) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", "", false
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.SplitN(trimmed, "/", 2)
		service := strings.TrimSpace(parts[0])
		account := strings.TrimSpace(parts[1])
		if service == "" || account == "" {
			return "", "", false
		}
		return service, account, true
	}
	return defaultKeychainService, trimmed, true
}

func isKeychainNotFound(output []byte) bool {
	if len(output) == 0 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(string(output)))
	switch {
	case strings.Contains(lower, "could not be found"):
		return true
	case strings.Contains(lower, "could not be found in the keychain"):
		return true
	case strings.Contains(lower, "the specified item could not be found"):
		return true
	default:
		return false
	}
}
