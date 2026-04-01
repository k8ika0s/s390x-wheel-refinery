package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
)

func TestInferPackRecipeDigests(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"openblas.sh": "echo openblas\n",
		"_common.sh":  "echo common\n",
		"versions.sh": "echo versions\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	cat := &pack.Catalog{
		Packs: map[string]pack.PackDef{
			"openblas": {Name: "openblas", Recipe: "openblas.sh"},
			"zlib":     {Name: "zlib", Recipe: "zlib.sh", RecipeDigest: "sha256:keep"},
		},
	}

	inferPackRecipeDigests(cat, dir)

	got := cat.Packs["openblas"].RecipeDigest
	if !strings.HasPrefix(got, "sha256:") || len(got) <= len("sha256:") {
		t.Fatalf("expected inferred digest, got %q", got)
	}
	if cat.Packs["zlib"].RecipeDigest != "sha256:keep" {
		t.Fatalf("expected existing digest preserved, got %q", cat.Packs["zlib"].RecipeDigest)
	}
}

func TestComputePackRecipeDigestMissingFile(t *testing.T) {
	dir := t.TempDir()
	if got := computePackRecipeDigest(dir, "missing.sh"); got != "" {
		t.Fatalf("expected empty digest for missing recipe, got %q", got)
	}
}
