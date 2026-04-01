package service

import (
	"strings"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

func TestPersistableResolvedHintSkipsDependencyPackFallback(t *testing.T) {
	hint := plan.Hint{
		ID:      "auto-openblas",
		Pattern: `Dependency "OpenBLAS" not found`,
		Recipes: map[string][]string{
			"dnf": {"openblas-devel"},
			"apt": {"libopenblas-dev"},
		},
	}
	resolution := remediationPlan{
		RemediationTier:  remediationTierDependencyPack,
		PackRequirements: []string{"openblas"},
		PackResolutionResult: map[string]any{
			"logical_dependencies": []string{"blas_lapack"},
			"selected_packs":       []string{"openblas"},
		},
	}
	persisted, reason := persistableResolvedHint(hint, resolution)
	if persisted != nil {
		t.Fatalf("expected no persisted hint for dependency-pack fallback, got %+v", persisted)
	}
	if !strings.Contains(reason, "dependency-pack fallback") {
		t.Fatalf("expected dependency-pack skip reason, got %q", reason)
	}
}

func TestSanitizePlanSnapshotDropsNativeHeavyBLASRepoRecipes(t *testing.T) {
	cfg := Config{
		PackCatalog: &pack.Catalog{
			Packs: map[string]pack.PackDef{
				"openblas": {Name: "openblas", Version: "0.3.25"},
			},
		},
	}
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{
			{
				Name:          "scikit-learn",
				Version:       "1.5.2",
				PythonVersion: "3.11",
				PythonTag:     "cp311",
				PlatformTag:   "manylinux2014_s390x",
				Action:        "build",
				Hints: []plan.HintMatch{
					{
						ID:      "auto-openblas",
						Pattern: `Dependency "OpenBLAS" not found, tried pkgconfig and cmake`,
						Recipes: map[string][]string{
							"dnf": {"openblas-devel"},
							"apt": {"libopenblas-dev"},
						},
					},
				},
				Recipes: []plan.RecipeMatch{
					{Name: "dnf:openblas-devel", Reason: "hint auto-openblas"},
					{Name: "apt:libopenblas-dev", Reason: "hint auto-openblas"},
					{Name: "dnf:gcc-toolset-12", Reason: "compiler toolset"},
				},
			},
		},
	}

	sanitizePlanSnapshot(cfg, &snap)

	gotRecipes := strings.Join(planRecipeNames(snap.Plan[0].Recipes), ",")
	if strings.Contains(gotRecipes, "openblas-devel") || strings.Contains(gotRecipes, "libopenblas-dev") {
		t.Fatalf("expected BLAS repo recipes removed from plan, got %q", gotRecipes)
	}
	if !strings.Contains(gotRecipes, "gcc-toolset-12") {
		t.Fatalf("expected unrelated compiler recipe preserved, got %q", gotRecipes)
	}
	if gotHintRecipes := strings.Join(flattenRecipeMap(snap.Plan[0].Hints[0].Recipes), ","); gotHintRecipes != "" {
		t.Fatalf("expected sanitized hint recipes to be empty after dependency-pack preference, got %q", gotHintRecipes)
	}
}
