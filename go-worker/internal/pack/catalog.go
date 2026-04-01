package pack

import (
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Catalog declares available packs and selection rules.
type Catalog struct {
	Packs map[string]PackDef `json:"packs" yaml:"packs"`
	Rules []Rule             `json:"rules" yaml:"rules"`
}

// PackDef describes a pack artifact/recipe.
type PackDef struct {
	Name         string   `json:"name" yaml:"name"`
	Version      string   `json:"version,omitempty" yaml:"version,omitempty"`
	RecipeDigest string   `json:"recipe_digest,omitempty" yaml:"recipe_digest,omitempty"`
	Recipe       string   `json:"recipe,omitempty" yaml:"recipe,omitempty"`
	Dependencies []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	// Optional description or notes.
	Note        string `json:"note,omitempty" yaml:"note,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Rule matches a package/build context to packs.
type Rule struct {
	Name            string   `json:"name,omitempty" yaml:"name,omitempty"`
	Description     string   `json:"description,omitempty" yaml:"description,omitempty"`
	PackagePattern  string   `json:"package_pattern,omitempty" yaml:"package_pattern,omitempty"` // substring/prefix match for now
	PackagePatterns []string `json:"package_patterns,omitempty" yaml:"package_patterns,omitempty"`
	Backend         string   `json:"backend,omitempty" yaml:"backend,omitempty"`
	Packs           []string `json:"packs" yaml:"packs"`
	Note            string   `json:"note,omitempty" yaml:"note,omitempty"`
}

// Load reads a worker pack catalog from disk.
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	for name, def := range catalog.Packs {
		if strings.TrimSpace(def.Name) == "" {
			def.Name = name
		}
		catalog.Packs[name] = def
	}
	return &catalog, nil
}

// Select returns packs for a package/backed based on simple pattern match.
func (c Catalog) Select(pkg string, backend string) []PackDef {
	var out []PackDef
	seen := make(map[string]struct{})
	lpkg := strings.ToLower(pkg)
	lbackend := strings.ToLower(backend)
	for _, r := range c.Rules {
		patterns := append([]string(nil), r.PackagePatterns...)
		if r.PackagePattern != "" {
			patterns = append(patterns, r.PackagePattern)
		}
		if len(patterns) > 0 && !matchesAnyPattern(lpkg, patterns) {
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

// GetPack returns a pack definition by logical name.
func (c Catalog) GetPack(name string) (PackDef, bool) {
	if c.Packs == nil {
		return PackDef{}, false
	}
	def, ok := c.Packs[strings.ToLower(strings.TrimSpace(name))]
	if ok {
		return def, true
	}
	for key, def := range c.Packs {
		if strings.EqualFold(key, name) || strings.EqualFold(def.Name, name) {
			return def, true
		}
	}
	return PackDef{}, false
}

// Dependencies returns direct dependencies for a pack.
func (c Catalog) Dependencies(name string) []string {
	def, ok := c.GetPack(name)
	if !ok || len(def.Dependencies) == 0 {
		return nil
	}
	return append([]string(nil), def.Dependencies...)
}

// ListPackNames returns sorted pack names from the catalog.
func (c Catalog) ListPackNames() []string {
	if len(c.Packs) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Packs))
	for _, def := range c.Packs {
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		names = append(names, def.Name)
	}
	sort.Strings(names)
	return names
}

func matchesAnyPattern(pkg string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if strings.Contains(pkg, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
