package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrReadOnlyProvider indicates the provider cannot be mutated via CLI.
var ErrReadOnlyProvider = errors.New("secret provider is read-only")

// Status reports secret lookup status without exposing values.
type Status struct {
	Provider string
	Key      string
	Found    bool
	ReadOnly bool
}

// Store manages secret references for supported providers.
type Store struct {
	secrets map[string]string
	getenv  func(string) string
}

// NewStore constructs a Store backed by the provided secrets map.
func NewStore(secrets map[string]string) Store {
	return Store{
		secrets: secrets,
		getenv:  os.Getenv,
	}
}

// WithEnv overrides the environment lookup (useful for tests).
func (s Store) WithEnv(getenv func(string) string) Store {
	if getenv != nil {
		s.getenv = getenv
	}
	return s
}

// Inspect returns the status for a secret reference.
func (s Store) Inspect(ref string) (Status, error) {
	provider, key, ok := ParseRef(ref)
	if !ok {
		return Status{}, ErrNotFound
	}
	status := Status{Provider: provider, Key: key}

	switch provider {
	case "plain", "literal", "raw":
		if strings.TrimSpace(key) == "" {
			return status, ErrNotFound
		}
		status.Found = true
		status.ReadOnly = true
		return status, nil
	case "secrets", "secret":
		if strings.TrimSpace(key) == "" {
			return status, ErrNotFound
		}
		if s.secrets == nil {
			return status, ErrNotFound
		}
		value, ok := s.secrets[key]
		if !ok || strings.TrimSpace(value) == "" {
			return status, ErrNotFound
		}
		status.Found = true
		return status, nil
	case "env":
		if strings.TrimSpace(key) == "" {
			return status, ErrNotFound
		}
		value := ""
		if s.getenv != nil {
			value = s.getenv(key)
		}
		if strings.TrimSpace(value) == "" {
			return status, ErrNotFound
		}
		status.Found = true
		status.ReadOnly = true
		return status, nil
	case "keychain", "1password", "op":
		if strings.TrimSpace(key) == "" {
			return status, ErrNotFound
		}
		switch provider {
		case "keychain":
			if _, err := keychainRead(key); err != nil {
				if errors.Is(err, ErrNotFound) {
					return status, ErrNotFound
				}
				return status, err
			}
			status.Found = true
			return status, nil
		case "1password", "op":
			if _, err := opRead(key); err != nil {
				if errors.Is(err, ErrNotFound) {
					return status, ErrNotFound
				}
				return status, err
			}
			status.Found = true
			status.ReadOnly = true
			return status, nil
		default:
			return status, fmtUnsupported(provider)
		}
	default:
		return status, fmtUnsupported(provider)
	}
}

// Set stores a secret value for a reference (secrets provider only).
func (s Store) Set(ref string, value string) (Status, error) {
	provider, key, ok := ParseRef(ref)
	if !ok {
		return Status{}, ErrNotFound
	}
	status := Status{Provider: provider, Key: key}

	switch provider {
	case "secrets", "secret":
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return status, ErrNotFound
		}
		if s.secrets == nil {
			return status, errors.New("secrets map is nil")
		}
		s.secrets[key] = value
		status.Found = true
		return status, nil
	case "plain", "literal", "raw", "env":
		status.ReadOnly = true
		return status, ErrReadOnlyProvider
	case "keychain", "1password", "op":
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return status, ErrNotFound
		}
		switch provider {
		case "keychain":
			if err := keychainWrite(key, value); err != nil {
				return status, err
			}
			status.Found = true
			return status, nil
		case "1password", "op":
			status.ReadOnly = true
			return status, ErrReadOnlyProvider
		default:
			return status, fmtUnsupported(provider)
		}
	default:
		return status, fmtUnsupported(provider)
	}
}

// Clear removes a secret value for a reference (secrets provider only).
func (s Store) Clear(ref string) (Status, error) {
	provider, key, ok := ParseRef(ref)
	if !ok {
		return Status{}, ErrNotFound
	}
	status := Status{Provider: provider, Key: key}

	switch provider {
	case "secrets", "secret":
		if strings.TrimSpace(key) == "" {
			return status, ErrNotFound
		}
		if s.secrets == nil {
			return status, ErrNotFound
		}
		if _, ok := s.secrets[key]; !ok {
			return status, ErrNotFound
		}
		delete(s.secrets, key)
		return status, nil
	case "plain", "literal", "raw", "env":
		status.ReadOnly = true
		return status, ErrReadOnlyProvider
	case "keychain", "1password", "op":
		if strings.TrimSpace(key) == "" {
			return status, ErrNotFound
		}
		switch provider {
		case "keychain":
			if err := keychainDelete(key); err != nil {
				if errors.Is(err, ErrNotFound) {
					return status, ErrNotFound
				}
				return status, err
			}
			return status, nil
		case "1password", "op":
			status.ReadOnly = true
			return status, ErrReadOnlyProvider
		default:
			return status, fmtUnsupported(provider)
		}
	default:
		return status, fmtUnsupported(provider)
	}
}

func fmtUnsupported(provider string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
}
