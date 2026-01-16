package store

import "testing"

func TestManifestDigestStableWithPackOrder(t *testing.T) {
	entry := ManifestEntry{
		Name:      "Demo",
		Version:   "1.0.0",
		WheelURL:  "https://example.com/demo-1.0.0.whl",
		PackURLs:  []string{"b", "a"},
		PythonTag: "cp311",
	}
	d1, err := ManifestDigest(entry)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	entry.PackURLs = []string{"a", "b"}
	d2, err := ManifestDigest(entry)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("expected stable digest, got %s and %s", d1, d2)
	}
}

func TestManifestDigestChangesOnField(t *testing.T) {
	entry := ManifestEntry{
		Name:      "Demo",
		Version:   "1.0.0",
		WheelURL:  "https://example.com/demo-1.0.0.whl",
		PythonTag: "cp311",
	}
	d1, err := ManifestDigest(entry)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	entry.Version = "1.0.1"
	d2, err := ManifestDigest(entry)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if d1 == d2 {
		t.Fatalf("expected digest change when version changes")
	}
}
