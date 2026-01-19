package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DisablePlatforms sets platforms.<name>.enabled=false and records the reason.
func DisablePlatforms(path string, issues map[string]string) (bool, error) {
	if strings.TrimSpace(path) == "" || len(issues) == 0 {
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, err
	}

	cfg, ok := coerceStringMap(raw)
	if !ok {
		return false, fmt.Errorf("config root must be a mapping/object")
	}

	platforms, ok := coerceStringMap(cfg["platforms"])
	if !ok {
		platforms = map[string]any{}
		cfg["platforms"] = platforms
	}

	changed := false
	now := time.Now().Format(time.RFC3339)
	for platform, reason := range issues {
		name := strings.TrimSpace(platform)
		if name == "" {
			continue
		}
		entry, ok := coerceStringMap(platforms[name])
		if !ok {
			entry = map[string]any{}
			platforms[name] = entry
			changed = true
		}

		if !isExplicitFalse(entry["enabled"]) {
			entry["enabled"] = false
			changed = true
		}

		trimmedReason := strings.TrimSpace(reason)
		if trimmedReason == "" {
			trimmedReason = "access unavailable"
		}
		if entry["disabled_reason"] != trimmedReason {
			entry["disabled_reason"] = trimmedReason
			changed = true
		}
		if entry["disabled_at"] != now {
			entry["disabled_at"] = now
			changed = true
		}
	}

	if !changed {
		return false, nil
	}

	output, err := yaml.Marshal(cfg)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, output, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func isExplicitFalse(value any) bool {
	switch typed := value.(type) {
	case bool:
		return !typed
	case string:
		normalized := strings.TrimSpace(strings.ToLower(typed))
		return normalized == "false" || normalized == "no" || normalized == "0"
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case float64:
		return typed == 0
	default:
		return false
	}
}

func coerceStringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := map[string]any{}
		for key, val := range typed {
			str, ok := key.(string)
			if !ok {
				continue
			}
			out[str] = val
		}
		return out, true
	default:
		return nil, false
	}
}
