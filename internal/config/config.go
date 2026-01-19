package config

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrNotFound = errors.New("config file not found")

// Config represents the user-facing YAML configuration.
type Config struct {
	Version    int             `yaml:"version"`
	Global     GlobalConfig    `yaml:"global"`
	Email      EmailConfig     `yaml:"email"`
	Platforms  PlatformsConfig `yaml:"platforms"`
	Tags       []TagConfig     `yaml:"tags"`
	Categories []TagConfig     `yaml:"categories"`
	Secrets    SecretsConfig   `yaml:"secrets"`
	Logging    LoggingConfig   `yaml:"logging"`
	Storage    StorageConfig   `yaml:"storage"`
}

// GlobalConfig holds global defaults.
type GlobalConfig struct {
	DefaultSchedule string       `yaml:"default_schedule"`
	Languages       []string     `yaml:"languages"`
	Digest          DigestConfig `yaml:"digest"`
}

// DigestConfig controls digest sizing.
type DigestConfig struct {
	WordCap  int `yaml:"word_cap"`
	MaxItems int `yaml:"max_items"`
}

// EmailConfig holds email delivery settings.
type EmailConfig struct {
	Provider string       `yaml:"provider"`
	From     string       `yaml:"from"`
	To       []string     `yaml:"to"`
	Subject  string       `yaml:"subject"`
	Resend   ResendConfig `yaml:"resend"`
	SMTP     SMTPConfig   `yaml:"smtp"`
}

// ResendConfig describes Resend provider settings.
type ResendConfig struct {
	APIKeyRef string `yaml:"api_key_ref"`
}

// SMTPConfig describes SMTP provider settings.
type SMTPConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	PasswordRef string `yaml:"password_ref"`
}

// SecretsConfig holds optional plaintext secrets (fallback only).
type SecretsConfig struct {
	Values map[string]string `yaml:"values"`
}

// PlatformsConfig groups platform configs.
type PlatformsConfig struct {
	Reddit    *PlatformConfig `yaml:"reddit"`
	X         *PlatformConfig `yaml:"x"`
	Instagram *PlatformConfig `yaml:"instagram"`
	HN        *PlatformConfig `yaml:"hn"`
}

// PlatformConfig describes a platform entry.
type PlatformConfig struct {
	Enabled        bool                          `yaml:"enabled"`
	DisabledReason string                        `yaml:"disabled_reason"`
	DisabledAt     string                        `yaml:"disabled_at"`
	Weight         float64                       `yaml:"weight"`
	CredentialsRef string                        `yaml:"credentials_ref"`
	Sources        map[string][]string           `yaml:"sources"`
	SourceWeights  map[string]map[string]float64 `yaml:"source_weights"`
}

// TagConfig describes tags or categories.
type TagConfig struct {
	Name       string   `yaml:"name"`
	Keywords   []string `yaml:"keywords"`
	Schedule   string   `yaml:"schedule"`
	Language   []string `yaml:"language"`
	Recipients []string `yaml:"recipients"`
	Weight     float64  `yaml:"weight"`
}

// LoggingConfig describes log retention.
type LoggingConfig struct {
	Level         string `yaml:"level"`
	RetentionDays int    `yaml:"retention_days"`
}

// StorageConfig describes storage paths and retention.
type StorageConfig struct {
	Path              string `yaml:"path"`
	SeenRetentionDays int    `yaml:"seen_retention_days"`
}

// Load reads config YAML from disk.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, ErrNotFound
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// FindTag returns the tag with the given name, case-insensitive.
func (c Config) FindTag(name string) (TagConfig, bool) {
	for _, tag := range c.Tags {
		if equalFold(tag.Name, name) {
			return tag, true
		}
	}
	return TagConfig{}, false
}

// FindCategory returns the category with the given name, case-insensitive.
func (c Config) FindCategory(name string) (TagConfig, bool) {
	for _, cat := range c.Categories {
		if equalFold(cat.Name, name) {
			return cat, true
		}
	}
	return TagConfig{}, false
}

func equalFold(a string, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return strings.EqualFold(a, b)
}
