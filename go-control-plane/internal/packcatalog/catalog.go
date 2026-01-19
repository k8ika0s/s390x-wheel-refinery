package packcatalog

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Catalog represents the pack catalog structure
type Catalog struct {
	Version  string             `yaml:"version" json:"version"`
	Updated  string             `yaml:"updated" json:"updated"`
	Packs    map[string]PackDef `yaml:"packs" json:"packs"`
	Runtimes map[string]Runtime `yaml:"runtimes" json:"runtimes"`
	Rules    []SelectionRule    `yaml:"rules" json:"rules"`
	mu       sync.RWMutex       `yaml:"-" json:"-"`
}

// PackDef defines a pack with its dependencies
type PackDef struct {
	Name         string   `yaml:"name" json:"name"`
	Version      string   `yaml:"version" json:"version"`
	Description  string   `yaml:"description,omitempty" json:"description,omitempty"`
	Recipe       string   `yaml:"recipe" json:"recipe"`
	Dependencies []string `yaml:"dependencies" json:"dependencies"`
}

// Runtime defines a Python runtime with its dependencies
type Runtime struct {
	Name          string   `yaml:"name" json:"name"`
	Version       string   `yaml:"version" json:"version"`
	PythonVersion string   `yaml:"python_version" json:"python_version"`
	Description   string   `yaml:"description,omitempty" json:"description,omitempty"`
	Recipe        string   `yaml:"recipe" json:"recipe"`
	Dependencies  []string `yaml:"dependencies" json:"dependencies"`
}

// SelectionRule defines automatic pack selection based on package patterns
type SelectionRule struct {
	Name            string   `yaml:"name" json:"name"`
	Description     string   `yaml:"description,omitempty" json:"description,omitempty"`
	PackagePatterns []string `yaml:"package_patterns" json:"package_patterns"`
	Packs           []string `yaml:"packs" json:"packs"`
}

// Load reads and parses the pack catalog from a YAML file
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	// Validate catalog
	if err := catalog.Validate(); err != nil {
		return nil, fmt.Errorf("validate catalog: %w", err)
	}

	return &catalog, nil
}

// Validate checks the catalog for consistency
func (c *Catalog) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check for circular dependencies in packs
	for name, pack := range c.Packs {
		if err := c.checkCircularDeps(name, pack.Dependencies, make(map[string]bool)); err != nil {
			return fmt.Errorf("pack %s: %w", name, err)
		}
	}

	// Check for circular dependencies in runtimes
	for name, runtime := range c.Runtimes {
		if err := c.checkCircularDeps(name, runtime.Dependencies, make(map[string]bool)); err != nil {
			return fmt.Errorf("runtime %s: %w", name, err)
		}
	}

	// Validate that all dependencies exist
	for name, pack := range c.Packs {
		for _, dep := range pack.Dependencies {
			if _, exists := c.Packs[dep]; !exists {
				return fmt.Errorf("pack %s references unknown dependency: %s", name, dep)
			}
		}
	}

	for name, runtime := range c.Runtimes {
		for _, dep := range runtime.Dependencies {
			if _, exists := c.Packs[dep]; !exists {
				return fmt.Errorf("runtime %s references unknown pack dependency: %s", name, dep)
			}
		}
	}

	return nil
}

// checkCircularDeps detects circular dependencies
func (c *Catalog) checkCircularDeps(name string, deps []string, visited map[string]bool) error {
	if visited[name] {
		return fmt.Errorf("circular dependency detected: %s", name)
	}

	visited[name] = true
	defer delete(visited, name)

	for _, dep := range deps {
		if pack, exists := c.Packs[dep]; exists {
			if err := c.checkCircularDeps(dep, pack.Dependencies, visited); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetPackDependencies returns all dependencies for a pack (transitive)
func (c *Catalog) GetPackDependencies(name string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	visited := make(map[string]bool)
	var result []string
	c.collectDeps(name, visited, &result)
	return result
}

// collectDeps recursively collects dependencies
func (c *Catalog) collectDeps(name string, visited map[string]bool, result *[]string) {
	if visited[name] {
		return
	}
	visited[name] = true

	pack, exists := c.Packs[name]
	if !exists {
		return
	}

	for _, dep := range pack.Dependencies {
		c.collectDeps(dep, visited, result)
	}

	*result = append(*result, name)
}

// GetRuntimeDependencies returns all dependencies for a runtime (transitive)
func (c *Catalog) GetRuntimeDependencies(name string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	runtime, exists := c.Runtimes[name]
	if !exists {
		return nil
	}

	visited := make(map[string]bool)
	var result []string

	for _, dep := range runtime.Dependencies {
		c.collectDeps(dep, visited, &result)
	}

	return result
}

// SelectPacksForPackage returns packs that should be used for a given package
func (c *Catalog) SelectPacksForPackage(packageName string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var selected []string
	seen := make(map[string]bool)
	pkgLower := strings.ToLower(packageName)

	for _, rule := range c.Rules {
		matched := false
		for _, pattern := range rule.PackagePatterns {
			if strings.Contains(pkgLower, strings.ToLower(pattern)) {
				matched = true
				break
			}
		}

		if matched {
			for _, pack := range rule.Packs {
				if !seen[pack] {
					seen[pack] = true
					selected = append(selected, pack)
				}
			}
		}
	}

	sort.Strings(selected)
	return selected
}

// GetPack returns a pack definition by name
func (c *Catalog) GetPack(name string) (PackDef, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pack, exists := c.Packs[name]
	return pack, exists
}

// GetRuntime returns a runtime definition by name
func (c *Catalog) GetRuntime(name string) (Runtime, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	runtime, exists := c.Runtimes[name]
	return runtime, exists
}

// ListPacks returns all pack names
func (c *Catalog) ListPacks() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.Packs))
	for name := range c.Packs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListRuntimes returns all runtime names
func (c *Catalog) ListRuntimes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.Runtimes))
	for name := range c.Runtimes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ToLegacyFormat converts catalog to the legacy pack.Catalog format
func (c *Catalog) ToLegacyFormat() map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]string)

	// Add pack dependencies
	for name, pack := range c.Packs {
		result[name] = pack.Dependencies
	}

	// Add runtime dependencies
	for name, runtime := range c.Runtimes {
		result[name] = runtime.Dependencies
		// Also add common aliases
		if runtime.PythonVersion != "" {
			result["cpython"] = runtime.Dependencies
			result["runtime"] = runtime.Dependencies
		}
	}

	return result
}

// Made with Bob
