package runner

import (
	"context"
	"testing"
	"time"
)

func TestFakeRunner(t *testing.T) {
	r := &FakeRunner{Dur: 50 * time.Millisecond, Log: "ok"}
	dur, logContent, err := r.Run(context.Background(), Job{Name: "pkg", Version: "1.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logContent != "ok" {
		t.Fatalf("unexpected log: %s", logContent)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
	if dur != 50*time.Millisecond {
		t.Fatalf("unexpected duration: %v", dur)
	}
}
