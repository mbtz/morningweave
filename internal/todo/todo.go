package todo

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"morningweave/internal/config"
	"morningweave/internal/scaffold"
	"morningweave/internal/secrets"
)

// MissingItem describes a missing configuration entry for USER_TODO.
type MissingItem struct {
	Text string
}

// CollectMissing inspects config and returns missing secret-related items.
func CollectMissing(cfg config.Config) []MissingItem {
	missing := []MissingItem{}
	seen := map[string]struct{}{}
	add := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if _, ok := seen[text]; ok {
			return
		}
		seen[text] = struct{}{}
		missing = append(missing, MissingItem{Text: text})
	}

	store := secrets.NewStore(cfg.Secrets.Values)
	provider := strings.ToLower(strings.TrimSpace(cfg.Email.Provider))
	if provider == "" {
		add("Set email.provider to resend or smtp.")
	} else {
		switch provider {
		case "resend":
			addMissingSecret(&missing, addMissingInput{
				Label: "email.resend.api_key_ref",
				Ref:   cfg.Email.Resend.APIKeyRef,
				Store: store,
				Hint:  "Resend API key reference",
			})
		case "smtp":
			addMissingSecret(&missing, addMissingInput{
				Label: "email.smtp.password_ref",
				Ref:   cfg.Email.SMTP.PasswordRef,
				Store: store,
				Hint:  "SMTP password reference",
			})
		default:
			add(fmt.Sprintf("Set email.provider to a supported value (resend or smtp). Current: %s.", provider))
		}
	}

	platforms := []struct {
		Name       string
		Cfg        *config.PlatformConfig
		NeedsCreds bool
	}{
		{Name: "reddit", Cfg: cfg.Platforms.Reddit, NeedsCreds: true},
		{Name: "x", Cfg: cfg.Platforms.X, NeedsCreds: true},
		{Name: "instagram", Cfg: cfg.Platforms.Instagram, NeedsCreds: true},
		{Name: "hn", Cfg: cfg.Platforms.HN, NeedsCreds: false},
	}
	requirements := platformAuthRequirements()

	for _, platform := range platforms {
		if platform.Cfg == nil || !platform.Cfg.Enabled || !platform.NeedsCreds {
			continue
		}
		req := requirements[platform.Name]
		addMissingSecret(&missing, addMissingInput{
			Label:       fmt.Sprintf("platforms.%s.credentials_ref", platform.Name),
			Ref:         platform.Cfg.CredentialsRef,
			Store:       store,
			Hint:        fmt.Sprintf("%s credentials reference", platform.Name),
			Requirement: req,
		})
	}

	return missing
}

type addMissingInput struct {
	Label       string
	Ref         string
	Store       secrets.Store
	Hint        string
	Requirement authRequirement
}

func addMissingSecret(target *[]MissingItem, input addMissingInput) {
	ref := strings.TrimSpace(input.Ref)
	if ref == "" {
		message := fmt.Sprintf("Set %s", input.Label)
		if hint := formatMissingHint(input.Hint, input.Requirement); hint != "" {
			message = fmt.Sprintf("%s (%s)", message, hint)
		}
		*target = append(*target, MissingItem{Text: message + "."})
		return
	}
	requirementSuffix := formatRequirementSuffix(input.Requirement)
	status, err := input.Store.Inspect(ref)
	if err == nil {
		if status.Found {
			return
		}
		*target = append(*target, MissingItem{Text: fmt.Sprintf("Store secret for %s (ref %s%s).", input.Label, ref, requirementSuffix)})
		return
	}

	switch {
	case errors.Is(err, secrets.ErrNotFound):
		*target = append(*target, MissingItem{Text: fmt.Sprintf("Store secret for %s (ref %s%s).", input.Label, ref, requirementSuffix)})
	case errors.Is(err, secrets.ErrProviderUnavailable):
		*target = append(*target, MissingItem{Text: fmt.Sprintf("Secret provider unavailable for %s (ref %s%s). Install provider CLI or switch to secrets:<key>.", input.Label, ref, requirementSuffix)})
	case errors.Is(err, secrets.ErrUnsupportedProvider):
		*target = append(*target, MissingItem{Text: fmt.Sprintf("Unsupported secret provider for %s (ref %s%s). Use secrets:<key> or keychain:<key>.", input.Label, ref, requirementSuffix)})
	default:
		*target = append(*target, MissingItem{Text: fmt.Sprintf("Secret check failed for %s (ref %s%s): %v", input.Label, ref, requirementSuffix, err)})
	}
}

func formatMissingHint(hint string, requirement authRequirement) string {
	base := strings.TrimSpace(hint)
	requirementHint := authRequirementHint(requirement)
	switch {
	case base == "" && requirementHint == "":
		return ""
	case base == "":
		return requirementHint
	case requirementHint == "":
		return base
	default:
		return base + "; " + requirementHint
	}
}

func formatRequirementSuffix(requirement authRequirement) string {
	hint := authRequirementHint(requirement)
	if hint == "" {
		return ""
	}
	return "; " + hint
}

// UpdateMissingSection updates USER_TODO with missing configuration entries.
func UpdateMissingSection(path string, emailProvider string, missing []MissingItem) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		content = []byte(scaffold.DefaultUserTodo(emailProvider))
	}

	updated := ensureMissingSection(string(content), missing)
	if updated == string(content) {
		return nil
	}

	return os.WriteFile(path, []byte(updated), 0o644)
}

const missingHeader = "## Missing configuration"

func ensureMissingSection(content string, missing []MissingItem) string {
	lines := splitLines(content)
	lines = removeMissingSection(lines)
	if len(missing) == 0 {
		return strings.Join(lines, "\n")
	}

	section := buildMissingSection(missing)
	insertAt := firstSectionIndex(lines)
	result := make([]string, 0, len(lines)+len(section)+2)
	result = append(result, lines[:insertAt]...)
	if len(result) > 0 && result[len(result)-1] != "" {
		result = append(result, "")
	}
	result = append(result, section...)
	if insertAt < len(lines) && (len(result) == 0 || result[len(result)-1] != "") {
		result = append(result, "")
	}
	result = append(result, lines[insertAt:]...)
	return strings.Join(result, "\n")
}

func buildMissingSection(missing []MissingItem) []string {
	section := []string{missingHeader, "Auto-generated from `morningweave status`/`run`.", ""}
	for _, item := range missing {
		section = append(section, fmt.Sprintf("- [ ] %s", item.Text))
	}
	return section
}

func removeMissingSection(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	result := make([]string, 0, len(lines))
	skipping := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, missingHeader) {
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(trimmed, "## ") {
				skipping = false
				result = append(result, line)
			}
			continue
		}
		result = append(result, line)
	}
	return trimTrailingBlanks(result)
}

func firstSectionIndex(lines []string) int {
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			return i
		}
	}
	return len(lines)
}

func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

func trimTrailingBlanks(lines []string) []string {
	for len(lines) > 0 {
		if strings.TrimSpace(lines[len(lines)-1]) != "" {
			break
		}
		lines = lines[:len(lines)-1]
	}
	return lines
}
