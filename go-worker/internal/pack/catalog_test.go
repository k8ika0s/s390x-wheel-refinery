package pack

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCatalogSelect(t *testing.T) {
	cat := Catalog{
		Packs: map[string]PackDef{
			"openssl": {Name: "openssl", Version: "3.0"},
			"rust":    {Name: "rust", Version: "1.76"},
		},
		Rules: []Rule{
			{PackagePattern: "crypt", Packs: []string{"openssl"}},
			{PackagePattern: "foo", Backend: "setuptools", Packs: []string{"openssl", "rust"}},
			{PackagePattern: "foo", Backend: "setuptools", Packs: []string{"openssl"}}, // duplicate should be filtered
			{PackagePattern: "foo", Backend: "maturin", Packs: []string{"rust"}},
			{PackagePatterns: []string{"bar", "baz"}, Backend: "setuptools", Packs: []string{"openssl"}},
			{PackagePattern: "missing", Packs: []string{"not_in_catalog"}}, // ignored
		},
	}

	tests := []struct {
		name     string
		pkg      string
		backend  string
		expected []string
	}{
		{
			name:     "case-insensitive pattern match",
			pkg:      "Cryptography",
			backend:  "",
			expected: []string{"openssl"},
		},
		{
			name:     "backend constrained with dedupe",
			pkg:      "foo-bar",
			backend:  "setuptools",
			expected: []string{"openssl", "rust"},
		},
		{
			name:     "backend mismatch skips rules",
			pkg:      "foo-bar",
			backend:  "maturin",
			expected: []string{"rust"},
		},
		{
			name:     "no matching rules",
			pkg:      "other",
			backend:  "setuptools",
			expected: nil,
		},
		{
			name:     "package_patterns yaml shape works too",
			pkg:      "bar-tool",
			expected: []string{"openssl"},
			backend:  "setuptools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs := cat.Select(tt.pkg, tt.backend)
			got := packNames(defs)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("Select(%q, %q)=%v, expected %v", tt.pkg, tt.backend, got, tt.expected)
			}
		})
	}
}

func TestLoadCatalogSupportsPackagePatternsAndDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pack-catalog.yaml")
	data := []byte(`
packs:
  openblas:
    version: "0.3.25"
    recipe: "openblas.sh"
    dependencies: []
rules:
  - package_patterns: ["scikit", "scipy"]
    packs: ["openblas"]
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cat == nil {
		t.Fatal("expected catalog")
	}
	def, ok := cat.GetPack("openblas")
	if !ok {
		t.Fatal("expected openblas pack")
	}
	if def.Recipe != "openblas.sh" {
		t.Fatalf("expected recipe loaded, got %q", def.Recipe)
	}
	got := packNames(cat.Select("scikit-learn", ""))
	if !reflect.DeepEqual(got, []string{"openblas"}) {
		t.Fatalf("expected selection via package_patterns, got %v", got)
	}
}

func packNames(defs []PackDef) []string {
	var names []string
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}
