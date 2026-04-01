package service

import (
	"strings"
	"testing"
	"time"

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

	ok, reason := w.canApplyAutoFix(job, "sig-b")
	if ok || !strings.Contains(reason, "rate limit") {
		t.Fatalf("expected rate limit block, got ok=%v reason=%q", ok, reason)
	}

	w.autoFixState[key] = autoFixState{lastApplied: now.Add(-40 * time.Minute), lastSignature: "sig-a"}
	ok, reason = w.canApplyAutoFix(job, "sig-a")
	if ok || !strings.Contains(reason, "duplicate") {
		t.Fatalf("expected duplicate block, got ok=%v reason=%q", ok, reason)
	}

	ok, reason = w.canApplyAutoFix(job, "sig-b")
	if !ok || reason != "" {
		t.Fatalf("expected allow after cooldown, got ok=%v reason=%q", ok, reason)
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
		Package:       "pandas",
		Version:       "2.2.3",
		PythonVersion: "3.11",
		PlatformTag:   "manylinux2014_s390x",
	}
	logContent := "error: subprocess-exited-with-error\nNumPy requires GCC >= 9.3"
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
	want := strings.Join([]string{
		"dnf:gcc-toolset-12",
		"dnf:gcc-toolset-12-gcc",
		"dnf:gcc-toolset-12-gcc-c++",
		"env:CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc",
		"env:CXX=/opt/rh/gcc-toolset-12/root/usr/bin/g++",
		"env:NPY_ALLOW_BLAS_UNSAFE=1",
	}, ",")
	if got != want {
		t.Fatalf("unexpected recipes: got %q want %q", got, want)
	}
}
