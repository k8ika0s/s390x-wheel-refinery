package pack

import (
	"strings"
)

// Catalog declares available packs and selection rules.
type Catalog struct {
	Packs map[string]PackDef `json:"packs" yaml:"packs"`
	Rules []Rule             `json:"rules" yaml:"rules"`
}

// PackDef describes a pack artifact/recipe.
type PackDef struct {
	Name         string `json:"name" yaml:"name"`
	Version      string `json:"version,omitempty" yaml:"version,omitempty"`
	RecipeDigest string `json:"recipe_digest,omitempty" yaml:"recipe_digest,omitempty"`
	// Optional description or notes.
	Note string `json:"note,omitempty" yaml:"note,omitempty"`
}

// Rule matches a package/build context to packs.
type Rule struct {
	PackagePattern string   `json:"package_pattern" yaml:"package_pattern"` // substring/prefix match for now
	Backend        string   `json:"backend,omitempty" yaml:"backend,omitempty"`
	Packs          []string `json:"packs" yaml:"packs"`
	Note           string   `json:"note,omitempty" yaml:"note,omitempty"`
}

// Select returns packs for a package/backed based on simple pattern match.
func (c Catalog) Select(pkg string, backend string) []PackDef {
	var out []PackDef
	seen := make(map[string]struct{})
	lpkg := strings.ToLower(pkg)
	lbackend := strings.ToLower(backend)
	for _, r := range c.Rules {
		if r.PackagePattern != "" && !strings.Contains(lpkg, strings.ToLower(r.PackagePattern)) {
			continue
		}
		if r.Backend != "" && lbackend != strings.ToLower(r.Backend) {
			continue
		}
		for _, name := range r.Packs {
			if def, ok := c.Packs[name]; ok {
				if _, dup := seen[name]; dup {
					continue
				}
				seen[name] = struct{}{}
				out = append(out, def)
			}
		}
	}
	return out
}
