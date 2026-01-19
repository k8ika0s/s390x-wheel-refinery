package packcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	// Create a temporary catalog file
	tmpDir := t.TempDir()
	catalogPath := filepath.Join(tmpDir, "test-catalog.yaml")

	catalogYAML := `version: "1.0"
updated: "2026-01-19"

packs:
  zlib:
    name: zlib
    version: "1.3.1"
    description: "Compression library"
    recipe: "zlib.sh"
    dependencies: []
    
  openssl:
    name: openssl
    version: "3.2.0"
    description: "Cryptography library"
    recipe: "openssl.sh"
    dependencies:
      - zlib

runtimes:
  cpython3.11:
    name: cpython3.11
    version: "3.11.7"
    python_version: "3.11"
    description: "CPython 3.11"
    recipe: "cpython311.sh"
    dependencies:
      - openssl
      - zlib

rules:
  - name: "crypto-packages"
    description: "Packages requiring OpenSSL"
    package_patterns:
      - "cryptography"
      - "pyopenssl"
    packs:
      - openssl
`

	if err := os.WriteFile(catalogPath, []byte(catalogYAML), 0644); err != nil {
		t.Fatalf("Failed to write test catalog: %v", err)
	}

	catalog, err := Load(catalogPath)
	if err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	if catalog.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", catalog.Version)
	}

	if len(catalog.Packs) != 2 {
		t.Errorf("Expected 2 packs, got %d", len(catalog.Packs))
	}

	if len(catalog.Runtimes) != 1 {
		t.Errorf("Expected 1 runtime, got %d", len(catalog.Runtimes))
	}

	if len(catalog.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(catalog.Rules))
	}
}

func TestGetPackDependencies(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"zlib": {
				Name:         "zlib",
				Dependencies: []string{},
			},
			"openssl": {
				Name:         "openssl",
				Dependencies: []string{"zlib"},
			},
			"libpng": {
				Name:         "libpng",
				Dependencies: []string{"zlib"},
			},
			"freetype": {
				Name:         "freetype",
				Dependencies: []string{"libpng", "zlib"},
			},
		},
	}

	tests := []struct {
		name     string
		pack     string
		expected []string
	}{
		{
			name:     "no dependencies",
			pack:     "zlib",
			expected: []string{"zlib"},
		},
		{
			name:     "single dependency",
			pack:     "openssl",
			expected: []string{"zlib", "openssl"},
		},
		{
			name:     "transitive dependencies",
			pack:     "freetype",
			expected: []string{"zlib", "libpng", "freetype"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := catalog.GetPackDependencies(tt.pack)
			if len(deps) != len(tt.expected) {
				t.Errorf("Expected %d dependencies, got %d: %v", len(tt.expected), len(deps), deps)
				return
			}
			for i, dep := range deps {
				if dep != tt.expected[i] {
					t.Errorf("Expected dependency %s at position %d, got %s", tt.expected[i], i, dep)
				}
			}
		})
	}
}

func TestGetRuntimeDependencies(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"zlib": {
				Name:         "zlib",
				Dependencies: []string{},
			},
			"openssl": {
				Name:         "openssl",
				Dependencies: []string{"zlib"},
			},
			"libffi": {
				Name:         "libffi",
				Dependencies: []string{},
			},
		},
		Runtimes: map[string]Runtime{
			"cpython3.11": {
				Name:         "cpython3.11",
				Dependencies: []string{"openssl", "libffi"},
			},
		},
	}

	deps := catalog.GetRuntimeDependencies("cpython3.11")
	expected := []string{"zlib", "openssl", "libffi"}

	if len(deps) != len(expected) {
		t.Errorf("Expected %d dependencies, got %d: %v", len(expected), len(deps), deps)
		return
	}

	// Check that all expected deps are present (order may vary)
	depsMap := make(map[string]bool)
	for _, dep := range deps {
		depsMap[dep] = true
	}

	for _, exp := range expected {
		if !depsMap[exp] {
			t.Errorf("Expected dependency %s not found in result", exp)
		}
	}
}

func TestSelectPacksForPackage(t *testing.T) {
	catalog := &Catalog{
		Rules: []SelectionRule{
			{
				Name:            "crypto-packages",
				PackagePatterns: []string{"cryptography", "pyopenssl"},
				Packs:           []string{"openssl", "libffi"},
			},
			{
				Name:            "imaging-packages",
				PackagePatterns: []string{"pillow", "pil"},
				Packs:           []string{"jpeg", "libpng", "freetype"},
			},
		},
	}

	tests := []struct {
		name     string
		pkg      string
		expected []string
	}{
		{
			name:     "crypto package",
			pkg:      "cryptography",
			expected: []string{"libffi", "openssl"},
		},
		{
			name:     "imaging package",
			pkg:      "Pillow",
			expected: []string{"freetype", "jpeg", "libpng"},
		},
		{
			name:     "no match",
			pkg:      "requests",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packs := catalog.SelectPacksForPackage(tt.pkg)
			if len(packs) != len(tt.expected) {
				t.Errorf("Expected %d packs, got %d: %v", len(tt.expected), len(packs), packs)
				return
			}
			for i, pack := range packs {
				if pack != tt.expected[i] {
					t.Errorf("Expected pack %s at position %d, got %s", tt.expected[i], i, pack)
				}
			}
		})
	}
}

func TestValidateCircularDependencies(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"a": {
				Name:         "a",
				Dependencies: []string{"b"},
			},
			"b": {
				Name:         "b",
				Dependencies: []string{"c"},
			},
			"c": {
				Name:         "c",
				Dependencies: []string{"a"}, // circular!
			},
		},
	}

	err := catalog.Validate()
	if err == nil {
		t.Error("Expected validation error for circular dependencies, got nil")
	}
}

func TestValidateUnknownDependency(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"openssl": {
				Name:         "openssl",
				Dependencies: []string{"zlib"}, // zlib doesn't exist
			},
		},
	}

	err := catalog.Validate()
	if err == nil {
		t.Error("Expected validation error for unknown dependency, got nil")
	}
}

func TestGetPack(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"zlib": {
				Name:    "zlib",
				Version: "1.3.1",
			},
		},
	}

	pack, exists := catalog.GetPack("zlib")
	if !exists {
		t.Error("Expected pack to exist")
	}
	if pack.Version != "1.3.1" {
		t.Errorf("Expected version 1.3.1, got %s", pack.Version)
	}

	_, exists = catalog.GetPack("nonexistent")
	if exists {
		t.Error("Expected pack to not exist")
	}
}

func TestListPacks(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"zlib":    {Name: "zlib"},
			"openssl": {Name: "openssl"},
			"libffi":  {Name: "libffi"},
		},
	}

	packs := catalog.ListPacks()
	expected := []string{"libffi", "openssl", "zlib"} // sorted

	if len(packs) != len(expected) {
		t.Errorf("Expected %d packs, got %d", len(expected), len(packs))
		return
	}

	for i, pack := range packs {
		if pack != expected[i] {
			t.Errorf("Expected pack %s at position %d, got %s", expected[i], i, pack)
		}
	}
}

func TestToLegacyFormat(t *testing.T) {
	catalog := &Catalog{
		Packs: map[string]PackDef{
			"zlib": {
				Name:         "zlib",
				Dependencies: []string{},
			},
			"openssl": {
				Name:         "openssl",
				Dependencies: []string{"zlib"},
			},
		},
		Runtimes: map[string]Runtime{
			"cpython3.11": {
				Name:          "cpython3.11",
				PythonVersion: "3.11",
				Dependencies:  []string{"openssl", "zlib"},
			},
		},
	}

	legacy := catalog.ToLegacyFormat()

	if len(legacy["openssl"]) != 1 || legacy["openssl"][0] != "zlib" {
		t.Errorf("Expected openssl to depend on zlib, got %v", legacy["openssl"])
	}

	if len(legacy["cpython3.11"]) != 2 {
		t.Errorf("Expected cpython3.11 to have 2 dependencies, got %d", len(legacy["cpython3.11"]))
	}

	// Check that runtime aliases are created
	if _, exists := legacy["cpython"]; !exists {
		t.Error("Expected 'cpython' alias to be created")
	}
	if _, exists := legacy["runtime"]; !exists {
		t.Error("Expected 'runtime' alias to be created")
	}
}

// Made with Bob
