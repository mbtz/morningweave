package todo

import (
	"strings"

	"morningweave/internal/connectors"
	"morningweave/internal/connectors/instagram"
	"morningweave/internal/connectors/reddit"
	"morningweave/internal/connectors/x"
)

type authRequirement struct {
	Required bool
	Scopes   []string
	Notes    string
}

func platformAuthRequirements() map[string]authRequirement {
	return map[string]authRequirement{
		"reddit":    toAuthRequirement(reddit.New().Requirements()),
		"x":         toAuthRequirement(x.New().Requirements()),
		"instagram": toAuthRequirement(instagram.New().Requirements()),
	}
}

func toAuthRequirement(req connectors.Requirements) authRequirement {
	auth := req.Auth
	scopes := append([]string(nil), auth.Scopes...)
	return authRequirement{
		Required: auth.Required,
		Scopes:   scopes,
		Notes:    strings.TrimSpace(auth.Notes),
	}
}

func authRequirementHint(req authRequirement) string {
	if !req.Required {
		return ""
	}
	parts := []string{}
	if len(req.Scopes) > 0 {
		parts = append(parts, "requires scopes: "+strings.Join(req.Scopes, ", "))
	}
	if note := strings.TrimSpace(req.Notes); note != "" {
		parts = append(parts, "note: "+note)
	}
	return strings.Join(parts, "; ")
}
