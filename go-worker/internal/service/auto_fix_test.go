package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

func TestAutoFixRateLimitAndDedupe(t *testing.T) {
	job := runner.Job{Name: "demo", Version: "1.0.0"}
	key := autoFixKey(job)
	if key == "" {
		t.Fatalf("expected auto-fix key")
	}
	now := time.Now()
	w := &Worker{
		Cfg: Config{AutoFixRateLimitMin: 30},
		autoFixState: map[string]autoFixState{
			key: {lastApplied: now.Add(-10 * time.Minute), lastSignature: "sig-a"},
		},
	}

	ok, reason := w.canApplyAutoFix(job, "sig-b", []string{"dnf:gcc-toolset-12"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if ok || !strings.Contains(reason, "rate limit") {
		t.Fatalf("expected rate limit block, got ok=%v reason=%q", ok, reason)
	}

	w.autoFixState[key] = autoFixState{lastApplied: now.Add(-40 * time.Minute), lastSignature: "sig-a"}
	ok, reason = w.canApplyAutoFix(job, "sig-a", []string{"dnf:gcc-toolset-12"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if ok || !strings.Contains(reason, "duplicate") {
		t.Fatalf("expected duplicate block, got ok=%v reason=%q", ok, reason)
	}

	ok, reason = w.canApplyAutoFix(job, "sig-b", []string{"dnf:gcc-toolset-12"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if !ok || reason != "" {
		t.Fatalf("expected allow after cooldown, got ok=%v reason=%q", ok, reason)
	}
}

func TestAutoFixCooldownBypassForLowRiskUtilityRecipes(t *testing.T) {
	job := runner.Job{Name: "demo", Version: "1.0.0"}
	key := autoFixKey(job)
	now := time.Now()
	w := &Worker{
		Cfg: Config{AutoFixRateLimitMin: 30},
		autoFixState: map[string]autoFixState{
			key: {lastApplied: now.Add(-2 * time.Minute), lastSignature: "sig-a"},
		},
	}
	ok, reason := w.canApplyAutoFix(job, "sig-b", []string{"dnf:findutils"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if !ok || reason != "" {
		t.Fatalf("expected low-risk utility fix to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}
	ok, reason = w.canApplyAutoFix(job, "sig-b", []string{"dnf:findutils", "env:PATH=/tmp"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if ok || !strings.Contains(reason, "rate limit") {
		t.Fatalf("expected env-bearing fix not to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}
	ok, reason = w.canApplyAutoFix(job, "sig-a", []string{"dnf:findutils"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if ok || !strings.Contains(reason, "duplicate") {
		t.Fatalf("expected duplicate signature to stay blocked, got ok=%v reason=%q", ok, reason)
	}

	ok, reason = w.canApplyAutoFix(job, "sig-c", []string{"env:LD_LIBRARY_PATH=/opt/runtime/lib:/opt/runtime/lib64:${LD_LIBRARY_PATH:-}"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if !ok || reason != "" {
		t.Fatalf("expected runtime libpath fix to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}

	ok, reason = w.canApplyAutoFix(job, "sig-d", []string{"dnf:openblas-devel"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if !ok || reason != "" {
		t.Fatalf("expected low-risk system library fix to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}

	ok, reason = w.canApplyAutoFix(job, "sig-e", []string{"apt:libopenblas-dev"}, nil, "heuristic", failureReason{}, remediationTierRepoPackage)
	if !ok || reason != "" {
		t.Fatalf("expected apt low-risk system library fix to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}
}

func TestAutoFixCooldownBypassForLLMPackageUnavailableRecovery(t *testing.T) {
	job := runner.Job{Name: "demo", Version: "1.0.0"}
	key := autoFixKey(job)
	now := time.Now()
	w := &Worker{
		Cfg: Config{AutoFixRateLimitMin: 30},
		autoFixState: map[string]autoFixState{
			key: {lastApplied: now.Add(-2 * time.Minute), lastSignature: "sig-a"},
		},
	}
	ok, reason := w.canApplyAutoFix(job, "sig-b", []string{"dnf:openblas", "dnf:gcc-gfortran"}, nil, "llm", failureReason{Code: "package_unavailable", Detail: "openblas-devel"}, remediationTierNormalizedAlternative)
	if !ok || reason != "" {
		t.Fatalf("expected llm package-unavailable recovery to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}
}

func TestAutoFixCooldownBypassForDependencyPackFallback(t *testing.T) {
	job := runner.Job{Name: "scikit-learn", Version: "1.5.2"}
	key := autoFixKey(job)
	now := time.Now()
	w := &Worker{
		Cfg: Config{AutoFixRateLimitMin: 30},
		autoFixState: map[string]autoFixState{
			key: {lastApplied: now.Add(-2 * time.Minute), lastSignature: "sig-a"},
		},
	}
	ok, reason := w.canApplyAutoFix(job, "sig-pack", nil, []string{"openblas"}, "heuristic", failureReason{Code: "package_unavailable", Detail: "openblas-devel"}, remediationTierDependencyPack)
	if !ok || reason != "" {
		t.Fatalf("expected dependency-pack fallback to bypass cooldown, got ok=%v reason=%q", ok, reason)
	}
}

func TestResolveRemediationPlanUsesPackFallbackForUnavailableBLASPackages(t *testing.T) {
	w := &Worker{Cfg: Config{PackCatalog: &pack.Catalog{
		Packs: map[string]pack.PackDef{
			"openblas": {Name: "openblas", Version: "0.3.25"},
		},
	}}}
	job := runner.Job{Name: "scikit-learn", Version: "1.5.2"}
	plan := w.resolveRemediationPlan(
		job,
		failureReason{Code: "package_unavailable", Detail: "openblas-devel"},
		[]string{"dnf:openblas-devel", "dnf:lapack-devel"},
		`Error: Unable to find a match: openblas-devel lapack-devel`,
	)
	if plan.RemediationTier != remediationTierDependencyPack {
		t.Fatalf("expected dependency-pack tier, got %q", plan.RemediationTier)
	}
	if got := strings.Join(plan.PackRequirements, ","); got != "openblas" {
		t.Fatalf("expected openblas fallback, got %q", got)
	}
	if len(plan.Recipes) != 0 {
		t.Fatalf("expected unavailable repo packages removed before pack fallback, got %v", plan.Recipes)
	}
}

func TestSanitizeRecipesForFailureDropsUnavailablePackages(t *testing.T) {
	recipes, dropped := sanitizeRecipesForFailure(
		[]string{"dnf:openblas-devel", "dnf:openblas", "apt:libopenblas-dev", "env:CC=/tmp/gcc"},
		failureReason{Code: "package_unavailable", Detail: "openblas-devel"},
		"No match for argument: openblas-devel\nError: Unable to find a match: openblas-devel",
	)
	got := strings.Join(recipes, ",")
	if strings.Contains(got, "dnf:openblas-devel") {
		t.Fatalf("expected unavailable dnf package dropped, got %q", got)
	}
	if !strings.Contains(got, "dnf:openblas") || !strings.Contains(got, "apt:libopenblas-dev") || !strings.Contains(got, "env:CC=/tmp/gcc") {
		t.Fatalf("expected remaining recipes preserved, got %q", got)
	}
	if drop := strings.Join(dropped, ","); !strings.Contains(drop, "dnf:openblas-devel") {
		t.Fatalf("expected dropped list to include unavailable package, got %q", drop)
	}
}

func TestInferHintFromLogMapsXargsToFindutils(t *testing.T) {
	hint, recipes, note, ok := inferHintFromLog(
		"/bin/sh: line 43: xargs: command not found",
		plan.HintContext{Package: "scikit-learn", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x"},
	)
	if !ok {
		t.Fatal("expected heuristic match")
	}
	if hint.Confidence != "medium" {
		t.Fatalf("expected medium confidence, got %q", hint.Confidence)
	}
	if !strings.Contains(note, "missing tool xargs") {
		t.Fatalf("unexpected note: %q", note)
	}
	got := strings.Join(recipes, ",")
	if !strings.Contains(got, "dnf:findutils") {
		t.Fatalf("expected dnf:findutils recipe, got %q", got)
	}
}

func TestInferHintFromLogMapsLibpythonLoadFailureToRuntimeLibPathFix(t *testing.T) {
	hint, recipes, note, ok := inferHintFromLog(
		"/opt/runtime/bin/python3: error while loading shared libraries: libpython3.11.so.1.0: cannot open shared object file: No such file or directory",
		plan.HintContext{Package: "scikit-learn", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x"},
	)
	if !ok {
		t.Fatal("expected heuristic match")
	}
	if hint.Confidence != "high" {
		t.Fatalf("expected high confidence, got %q", hint.Confidence)
	}
	if !strings.Contains(note, "runtime libpython shared library path") {
		t.Fatalf("unexpected note: %q", note)
	}
	got := strings.Join(recipes, ",")
	if !strings.Contains(got, "env:LD_LIBRARY_PATH=/opt/runtime/lib:/opt/runtime/lib64:${LD_LIBRARY_PATH:-}") {
		t.Fatalf("expected runtime libpath recipe, got %q", got)
	}
}

func TestInferHintFromLogMapsMesonDependencyOpenBLASToSystemLibraryRecipe(t *testing.T) {
	hint, recipes, note, ok := inferHintFromLog(
		`../scipy/meson.build:58:15: ERROR: Dependency "OpenBLAS" not found, tried pkgconfig and cmake`,
		plan.HintContext{Package: "scikit-learn", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x"},
	)
	if !ok {
		t.Fatal("expected heuristic match")
	}
	if hint.Confidence != "high" {
		t.Fatalf("expected high confidence, got %q", hint.Confidence)
	}
	if !strings.Contains(note, "missing Meson dependency OpenBLAS") {
		t.Fatalf("unexpected note: %q", note)
	}
	got := strings.Join(recipes, ",")
	if !strings.Contains(got, "dnf:openblas-devel") {
		t.Fatalf("expected dnf:openblas-devel recipe, got %q", got)
	}
	if !strings.Contains(got, "apt:libopenblas-dev") {
		t.Fatalf("expected apt:libopenblas-dev recipe, got %q", got)
	}
}

func TestCompactTracePreservesOrder(t *testing.T) {
	in := []string{"matched hint A", "", "blocked hint B", "matched hint A", "applied recipes"}
	out := compactTrace(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(out))
	}
	if out[0] != "matched hint A" || out[1] != "blocked hint B" || out[2] != "applied recipes" {
		t.Fatalf("unexpected trace order: %v", out)
	}
}

func TestInferHintFromLogPrefersCompilerToolsetForNumPyGCCMismatch(t *testing.T) {
	ctxHint := plan.HintContext{
		Package:       "scikit-learn",
		Version:       "1.5.2",
		PythonVersion: "3.11",
		PlatformTag:   "manylinux2014_s390x",
	}
	logContent := "../meson.build:25:4: ERROR: Problem encountered: NumPy requires GCC >= 9.3"
	hint, recipes, note, ok := inferHintFromLog(logContent, ctxHint)
	if !ok {
		t.Fatal("expected heuristic match")
	}
	if hint.Confidence != "high" {
		t.Fatalf("expected high confidence, got %q", hint.Confidence)
	}
	if !strings.Contains(note, "compiler toolset") {
		t.Fatalf("expected compiler toolset note, got %q", note)
	}
	got := strings.Join(recipes, ",")
	for _, want := range []string{
		"dnf:gcc-toolset-12",
		"dnf:gcc-toolset-12-gcc",
		"dnf:gcc-toolset-12-gcc-c++",
		"env:CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc",
		"env:CXX=/opt/rh/gcc-toolset-12/root/usr/bin/g++",
		"env:LD_LIBRARY_PATH=/opt/rh/gcc-toolset-12/root/usr/lib64:${LD_LIBRARY_PATH:-}",
		"env:NPY_ALLOW_BLAS_UNSAFE=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected recipes to contain %q, got %q", want, got)
		}
	}
}

func TestInferHintFromLogSkipsEncodingsRuntimeCorruption(t *testing.T) {
	_, recipes, _, ok := inferHintFromLog(
		"Fatal Python error: Py_Initialize: Unable to get the locale encoding\nModuleNotFoundError: No module named 'encodings'",
		plan.HintContext{Package: "pandas", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x"},
	)
	if ok {
		t.Fatalf("expected no inferred hint, got recipes=%v", recipes)
	}
}

func TestInferHintFromLogSkipsInitFsEncodingRuntimeCorruption(t *testing.T) {
	_, recipes, _, ok := inferHintFromLog(
		"Fatal Python error: init_fs_encoding: failed to get the Python codec of the filesystem encoding\nPython runtime state: core initialized\nModuleNotFoundError: No module named 'encodings'",
		plan.HintContext{Package: "scikit-learn", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x"},
	)
	if ok {
		t.Fatalf("expected no inferred hint, got recipes=%v", recipes)
	}
}

func TestAutoFixSkipsInfrastructureFailures(t *testing.T) {
	w := &Worker{Cfg: Config{AutoFixEnabled: true}}
	result := w.autoFix(
		context.Background(),
		runner.Job{Name: "scikit-learn", Version: "1.5.2", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x"},
		`Error: 127.0.0.1:5000/refinery-builder:latest: image not known`,
		nil,
		nil,
		failureReason{Code: "builder_image_missing", Detail: "refinery-builder"},
	)
	if result.Applied {
		t.Fatal("expected infrastructure failure not to auto-apply")
	}
	if result.BlockedReason == "" || !strings.Contains(result.BlockedReason, "infrastructure failure") {
		t.Fatalf("expected infrastructure block reason, got %q", result.BlockedReason)
	}
	if len(result.DecisionTrace) == 0 || !strings.Contains(strings.Join(result.DecisionTrace, "\n"), "infrastructure failure builder_image_missing") {
		t.Fatalf("expected infrastructure skip trace, got %v", result.DecisionTrace)
	}
}
