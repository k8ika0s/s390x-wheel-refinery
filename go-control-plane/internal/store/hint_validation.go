package store

import (
	"regexp"
	"strconv"
	"strings"
)

// NormalizeHint trims hint fields and removes empty entries.
func NormalizeHint(h Hint) Hint {
	h.ID = strings.TrimSpace(h.ID)
	h.Pattern = strings.TrimSpace(h.Pattern)
	h.Note = strings.TrimSpace(h.Note)
	h.Severity = strings.ToLower(strings.TrimSpace(h.Severity))
	h.Confidence = strings.ToLower(strings.TrimSpace(h.Confidence))
	h.Tags = trimStringSlice(h.Tags)
	h.Examples = trimStringSlice(h.Examples)
	h.Recipes = trimStringMap(h.Recipes)
	h.AppliesTo = trimStringMap(h.AppliesTo)
	return h
}

// ValidateHint returns validation errors for a hint.
func ValidateHint(h Hint) []string {
	var errs []string
	if h.ID == "" {
		errs = append(errs, "id required")
	}
	if h.Pattern == "" {
		errs = append(errs, "pattern required")
	} else if _, err := regexp.Compile(h.Pattern); err != nil {
		errs = append(errs, "pattern invalid: "+err.Error())
	}
	if h.Note == "" {
		errs = append(errs, "note required")
	}
	if !hasRecipes(h.Recipes) {
		errs = append(errs, "recipes required")
	}
	if h.Severity != "" {
		switch h.Severity {
		case "info", "warn", "warning", "error":
		default:
			errs = append(errs, "severity must be info|warn|warning|error")
		}
	}
	if h.Confidence != "" {
		switch h.Confidence {
		case "low", "medium", "high":
		default:
			if f, err := strconv.ParseFloat(h.Confidence, 64); err != nil || f < 0 || f > 1 {
				errs = append(errs, "confidence must be low|medium|high or 0..1")
			}
		}
	}
	for k, v := range h.AppliesTo {
		if strings.TrimSpace(k) == "" || len(v) == 0 {
			errs = append(errs, "applies_to entries must have non-empty keys and values")
			break
		}
	}
	return errs
}

func trimStringSlice(items []string) []string {
	var out []string
	for _, v := range items {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func trimStringMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		trimmed := trimStringSlice(v)
		if len(trimmed) == 0 {
			continue
		}
		out[key] = trimmed
	}
	return out
}

func hasRecipes(recipes map[string][]string) bool {
	for _, v := range recipes {
		if len(v) > 0 {
			return true
		}
	}
	return false
}
