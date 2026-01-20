package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrNotFound            = errors.New("secret not found")
	ErrUnsupportedProvider = errors.New("unsupported secret provider")
	ErrProviderUnavailable = errors.New("secret provider unavailable")
)

// Resolver resolves secret references from supported providers.
type Resolver struct {
	secrets map[string]string
	getenv  func(string) string
}

// NewResolver constructs a Resolver backed by the provided secrets map.
func NewResolver(secrets map[string]string) Resolver {
	return Resolver{
		secrets: secrets,
		getenv:  os.Getenv,
	}
}

// WithEnv overrides the environment lookup (useful for tests).
func (r Resolver) WithEnv(getenv func(string) string) Resolver {
	if getenv != nil {
		r.getenv = getenv
	}
	return r
}

// Resolve returns the secret value for the provided reference.
// Supported providers: secrets, env, plain.
func (r Resolver) Resolve(ref string) (string, error) {
	provider, key, ok := parseRef(ref)
	if !ok {
		return "", ErrNotFound
	}

	switch provider {
	case "plain", "literal", "raw":
		if strings.TrimSpace(key) == "" {
			return "", ErrNotFound
		}
		return key, nil
	case "secrets", "secret":
		if strings.TrimSpace(key) == "" {
			return "", ErrNotFound
		}
		if value, ok := r.secrets[key]; ok && strings.TrimSpace(value) != "" {
			return value, nil
		}
		return "", ErrNotFound
	case "env":
		if strings.TrimSpace(key) == "" {
			return "", ErrNotFound
		}
		value := ""
		if r.getenv != nil {
			value = r.getenv(key)
		}
		if strings.TrimSpace(value) == "" {
			return "", ErrNotFound
		}
		return value, nil
	case "keychain", "1password", "op":
		if strings.TrimSpace(key) == "" {
			return "", ErrNotFound
		}
		switch provider {
		case "keychain":
			value, err := keychainRead(key)
			if err != nil {
				return "", err
			}
			return value, nil
		case "1password", "op":
			value, err := opRead(key)
			if err != nil {
				return "", err
			}
			return value, nil
		default:
			return "", fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
		}
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}
}

// ParseRef splits a reference into provider and key.
func ParseRef(ref string) (string, string, bool) {
	return parseRef(ref)
}

func parseRef(ref string) (string, string, bool) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "op://") {
		return "op", "op://"+trimmed[len("op://"):], true
	}
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) == 1 {
		return "plain", trimmed, true
	}
	provider := strings.ToLower(strings.TrimSpace(parts[0]))
	key := strings.TrimSpace(parts[1])
	if provider == "" {
		provider = "plain"
	}
	if provider == "op" || provider == "1password" {
		key = normalizeOpKey(key)
	}
	return provider, key, true
}

func normalizeOpKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "op://") {
		return "op://" + trimmed[len("op://"):]
	}
	trimmed = strings.TrimPrefix(trimmed, "//")
	trimmed = strings.TrimPrefix(trimmed, "/")
	return "op://" + trimmed
}
