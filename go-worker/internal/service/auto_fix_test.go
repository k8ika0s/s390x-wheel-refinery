package service

import (
	"strings"
	"testing"
	"time"

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
