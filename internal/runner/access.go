package runner

import "strings"

// DetectAccessIssues extracts platform access issues from warning strings.
func DetectAccessIssues(warnings []string) map[string]string {
	if len(warnings) == 0 {
		return nil
	}

	issues := map[string]string{}
	for _, warning := range warnings {
		platform, reason, ok := parseAccessWarning(warning)
		if !ok {
			continue
		}
		if _, exists := issues[platform]; !exists {
			issues[platform] = reason
		}
	}

	if len(issues) == 0 {
		return nil
	}
	return issues
}

func parseAccessWarning(warning string) (string, string, bool) {
	trimmed := strings.TrimSpace(warning)
	if trimmed == "" {
		return "", "", false
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "x: api tier/access issue:"):
		return "x", strings.TrimSpace(trimmed[len("x: "):]), true
	case strings.HasPrefix(lower, "x: credentials rejected:"):
		return "x", strings.TrimSpace(trimmed[len("x: "):]), true
	case strings.HasPrefix(lower, "x:") && strings.Contains(lower, "fetch failed:") && containsAccessKeyword(lower):
		return "x", strings.TrimSpace(trimmed[len("x: "):]), true
	case strings.HasPrefix(lower, "reddit: fetch failed:") && containsAccessKeyword(lower):
		return "reddit", strings.TrimSpace(trimmed[len("reddit: "):]), true
	case strings.HasPrefix(lower, "instagram: fetch failed:") && containsAccessKeyword(lower):
		return "instagram", strings.TrimSpace(trimmed[len("instagram: "):]), true
	default:
		return "", "", false
	}
}

func containsAccessKeyword(value string) bool {
	keywords := []string{
		"auth error",
		"unauthorized",
		"forbidden",
		"oauth",
		"access token",
		"invalid token",
		"invalid_grant",
		"invalid client",
		"not authorized",
		"permission",
		"permissions",
		"access denied",
		"client-not-enrolled",
		"insufficient",
		"tier",
		"limited",
	}
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
