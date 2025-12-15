package pack

import (
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

func packNames(defs []PackDef) []string {
	var names []string
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}
