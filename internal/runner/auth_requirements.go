package runner

import (
	"fmt"
	"strings"

	"morningweave/internal/connectors"
	"morningweave/internal/connectors/hn"
	"morningweave/internal/connectors/instagram"
	"morningweave/internal/connectors/reddit"
	xconn "morningweave/internal/connectors/x"
)

// AuthRequirementHint formats connector auth requirements for warnings.
func AuthRequirementHint(platform string) string {
	req, ok := platformAuthRequirements(platform)
	if !ok || !req.Required {
		return ""
	}

	parts := []string{}
	if len(req.Scopes) > 0 {
		parts = append(parts, fmt.Sprintf("scopes: %s", strings.Join(req.Scopes, ", ")))
	}
	if notes := strings.TrimSpace(req.Notes); notes != "" {
		parts = append(parts, fmt.Sprintf("notes: %s", notes))
	}
	if len(parts) == 0 {
		return ""
	}

	name := strings.ToLower(strings.TrimSpace(platform))
	if name == "" {
		name = "platform"
	}
	return fmt.Sprintf("%s: auth requirements (%s)", name, strings.Join(parts, "; "))
}

// AppendAuthRequirementHint adds an auth requirement hint for the platform if not already present.
func AppendAuthRequirementHint(warnings []string, platform string) []string {
	hint := AuthRequirementHint(platform)
	if hint == "" {
		return warnings
	}
	for _, warning := range warnings {
		if warning == hint {
			return warnings
		}
	}
	return append(warnings, hint)
}

func platformAuthRequirements(platform string) (connectors.AuthRequirements, bool) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "reddit":
		return reddit.New().Requirements().Auth, true
	case "x":
		return xconn.New().Requirements().Auth, true
	case "instagram":
		return instagram.New().Requirements().Auth, true
	case "hn":
		return hn.New().Requirements().Auth, true
	default:
		return connectors.AuthRequirements{}, false
	}
}
