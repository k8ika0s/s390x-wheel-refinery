package plan

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Hint represents a hint catalog entry from the control-plane.
type Hint struct {
	ID         string              `json:"id"`
	Pattern    string              `json:"pattern"`
	Recipes    map[string][]string `json:"recipes,omitempty"`
	Note       string              `json:"note,omitempty"`
	Tags       []string            `json:"tags,omitempty"`
	Severity   string              `json:"severity,omitempty"`
	AppliesTo  map[string][]string `json:"applies_to,omitempty"`
	Confidence string              `json:"confidence,omitempty"`
	Examples   []string            `json:"examples,omitempty"`
}

// HintMatch captures a hint attached to a plan node.
type HintMatch struct {
	ID      string              `json:"id"`
	Pattern string              `json:"pattern,omitempty"`
	Note    string              `json:"note,omitempty"`
	Reason  string              `json:"reason,omitempty"`
	Tags    []string            `json:"tags,omitempty"`
	Recipes map[string][]string `json:"recipes,omitempty"`
}

// RecipeMatch is a flattened recipe reference for display.
type RecipeMatch struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type hintContext struct {
	Package       string
	Version       string
	PythonVersion string
	PythonTag     string
	PlatformTag   string
	BuildBackend  string
}

// AttachHints applies hint matches to plan nodes.
func AttachHints(snap *Snapshot, hints []Hint) {
	if snap == nil || len(hints) == 0 || len(snap.Plan) == 0 {
		return
	}
	for i, node := range snap.Plan {
		ctx := hintContext{
			Package:       node.Name,
			Version:       node.Version,
			PythonVersion: node.PythonVersion,
			PythonTag:     node.PythonTag,
			PlatformTag:   node.PlatformTag,
		}
		matches, recipes := matchHints(hints, ctx)
		if len(matches) == 0 && len(recipes) == 0 {
			continue
		}
		node.Hints = matches
		node.Recipes = recipes
		snap.Plan[i] = node
	}
}

func matchHints(hints []Hint, ctx hintContext) ([]HintMatch, []RecipeMatch) {
	var matches []HintMatch
	var recipes []RecipeMatch
	for _, h := range hints {
		match, recs, ok := matchHint(h, ctx)
		if !ok {
			continue
		}
		matches = append(matches, match)
		recipes = append(recipes, recs...)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].ID < matches[j].ID
	})
	recipes = dedupeRecipes(recipes)
	return matches, recipes
}

func matchHint(h Hint, ctx hintContext) (HintMatch, []RecipeMatch, bool) {
	pkg := normalizeName(ctx.Package)
	if pkg == "" {
		return HintMatch{}, nil, false
	}
	pyVersion := strings.TrimSpace(ctx.PythonVersion)
	if pyVersion == "" {
		pyVersion = pythonVersionFromTag(ctx.PythonTag)
	}
	platform := platformFamily(ctx.PlatformTag)
	arch := archFromPlatformTag(ctx.PlatformTag)
	matchedSpecific := false
	var reasons []string

	applies := normalizeAppliesTo(h.AppliesTo)

	if vals := valuesForKeys(applies, "packages", "package", "package_names"); len(vals) > 0 {
		if !matchExact(pkg, vals) {
			return HintMatch{}, nil, false
		}
		matchedSpecific = true
		reasons = append(reasons, fmt.Sprintf("package=%s", pkg))
	}
	if vals := valuesForKeys(applies, "package_patterns", "package_pattern"); len(vals) > 0 {
		if !matchAnyPattern(pkg, vals) {
			return HintMatch{}, nil, false
		}
		matchedSpecific = true
		reasons = append(reasons, "package pattern")
	}
	if vals := valuesForKeys(applies, "platforms", "platform"); len(vals) > 0 {
		if !matchPlatform(platform, ctx.PlatformTag, vals) {
			return HintMatch{}, nil, false
		}
		reasons = append(reasons, fmt.Sprintf("platform=%s", platform))
	}
	if vals := valuesForKeys(applies, "arch", "archs"); len(vals) > 0 {
		if !matchExact(arch, vals) {
			return HintMatch{}, nil, false
		}
		reasons = append(reasons, fmt.Sprintf("arch=%s", arch))
	}
	if vals := valuesForKeys(applies, "python_versions", "python_version"); len(vals) > 0 {
		if !matchExact(pyVersion, vals) {
			return HintMatch{}, nil, false
		}
		reasons = append(reasons, fmt.Sprintf("python=%s", pyVersion))
	}
	if vals := valuesForKeys(applies, "python_tags", "python_tag"); len(vals) > 0 {
		if !matchExact(strings.ToLower(ctx.PythonTag), vals) {
			return HintMatch{}, nil, false
		}
		reasons = append(reasons, fmt.Sprintf("tag=%s", ctx.PythonTag))
	}
	if vals := valuesForKeys(applies, "platform_tags", "platform_tag"); len(vals) > 0 {
		if !matchExact(strings.ToLower(ctx.PlatformTag), vals) {
			return HintMatch{}, nil, false
		}
		reasons = append(reasons, fmt.Sprintf("platform_tag=%s", ctx.PlatformTag))
	}
	if vals := valuesForKeys(applies, "build_backends", "build_backend"); len(vals) > 0 {
		if ctx.BuildBackend == "" || !matchExact(strings.ToLower(ctx.BuildBackend), vals) {
			return HintMatch{}, nil, false
		}
		matchedSpecific = true
		reasons = append(reasons, fmt.Sprintf("backend=%s", ctx.BuildBackend))
	}

	if !matchedSpecific && matchTags(pkg, h.Tags) {
		matchedSpecific = true
		reasons = append(reasons, "tag match")
	}

	if !matchedSpecific {
		return HintMatch{}, nil, false
	}

	match := HintMatch{
		ID:      h.ID,
		Pattern: h.Pattern,
		Note:    h.Note,
		Reason:  strings.Join(reasons, "; "),
		Tags:    h.Tags,
		Recipes: h.Recipes,
	}
	return match, recipesFromHint(h, match.Reason), true
}

func recipesFromHint(h Hint, reason string) []RecipeMatch {
	if len(h.Recipes) == 0 {
		return nil
	}
	var out []RecipeMatch
	keys := make([]string, 0, len(h.Recipes))
	for k := range h.Recipes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, mgr := range keys {
		steps := h.Recipes[mgr]
		for _, step := range steps {
			name := fmt.Sprintf("%s:%s", mgr, strings.TrimSpace(step))
			out = append(out, RecipeMatch{Name: name, Reason: fmt.Sprintf("hint %s: %s", h.ID, reason)})
		}
	}
	return out
}

func dedupeRecipes(in []RecipeMatch) []RecipeMatch {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]RecipeMatch)
	for _, r := range in {
		key := strings.ToLower(strings.TrimSpace(r.Name))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; !ok {
			seen[key] = r
		}
	}
	out := make([]RecipeMatch, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func normalizeAppliesTo(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" || len(v) == 0 {
			continue
		}
		var vals []string
		for _, raw := range v {
			t := strings.ToLower(strings.TrimSpace(raw))
			if t != "" {
				vals = append(vals, t)
			}
		}
		if len(vals) > 0 {
			out[key] = vals
		}
	}
	return out
}

func valuesForKeys(in map[string][]string, keys ...string) []string {
	if len(in) == 0 {
		return nil
	}
	var out []string
	for _, key := range keys {
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		if vals, ok := in[k]; ok {
			out = append(out, vals...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var uniq []string
	for _, v := range out {
		t := strings.ToLower(strings.TrimSpace(v))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		uniq = append(uniq, t)
	}
	return uniq
}

func matchExact(value string, allowed []string) bool {
	if value == "" {
		return false
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, v := range allowed {
		if value == strings.ToLower(strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

func matchAnyPattern(value string, patterns []string) bool {
	if value == "" {
		return false
	}
	for _, p := range patterns {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func matchPlatform(family, platformTag string, allowed []string) bool {
	if family == "" && platformTag == "" {
		return false
	}
	ltag := strings.ToLower(platformTag)
	for _, v := range allowed {
		if v == "" {
			continue
		}
		if family != "" && v == family {
			return true
		}
		if ltag != "" && strings.Contains(ltag, v) {
			return true
		}
	}
	return false
}

func matchTags(pkg string, tags []string) bool {
	if pkg == "" || len(tags) == 0 {
		return false
	}
	for _, tag := range tags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if t != "" && strings.Contains(pkg, t) {
			return true
		}
	}
	return false
}

func platformFamily(platformTag string) string {
	ltag := strings.ToLower(platformTag)
	switch {
	case strings.Contains(ltag, "linux"), strings.Contains(ltag, "manylinux"):
		return "linux"
	case strings.Contains(ltag, "darwin"), strings.Contains(ltag, "macos"), strings.Contains(ltag, "macosx"):
		return "darwin"
	case strings.Contains(ltag, "win"):
		return "windows"
	default:
		return ""
	}
}

func archFromPlatformTag(platformTag string) string {
	if platformTag == "" {
		return ""
	}
	parts := strings.Split(strings.ToLower(platformTag), "_")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func pythonVersionFromTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if strings.HasPrefix(tag, "cp") && len(tag) >= 4 {
		rest := strings.TrimPrefix(tag, "cp")
		if len(rest) >= 2 {
			return fmt.Sprintf("%s.%s", rest[:1], rest[1:2])
		}
	}
	if strings.HasPrefix(tag, "py") && len(tag) > 2 {
		return strings.TrimPrefix(tag, "py")
	}
	return ""
}
