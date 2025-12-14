package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

func TestWorkerDrainMatchesJobs(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	snap := plan.Snapshot{RunID: "r1", Plan: []plan.FlatNode{{Name: "pkg", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}}}
	data, _ := json.Marshal(snap)
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fq := queue.NewFileQueue(filepath.Join(dir, "queue.json"))
	_ = fq.Enqueue(context.Background(), queue.Request{Package: "pkg", Version: "1.0.0"})
	r := &runner.FakeRunner{Dur: 100 * time.Millisecond, Log: "ok"}
	w := &Worker{Queue: fq, Runner: r, Reporter: nil, Cfg: Config{OutputDir: dir, CacheDir: dir}}
	// preload plan
	if err := w.LoadPlan(); err != nil {
		t.Fatal(err)
	}
	if err := w.Drain(context.Background()); err != nil {
		t.Fatalf("drain error: %v", err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
}

func TestWorkerRunsPlanWhenQueueEmpty(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	snap := plan.Snapshot{RunID: "r1", Plan: []plan.FlatNode{
		{Name: "pkg", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"},
		{Name: "reuse", Version: "2.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "reuse"},
	}}
	data, _ := json.Marshal(snap)
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fq := queue.NewFileQueue(filepath.Join(dir, "queue.json"))
	r := &runner.FakeRunner{Dur: 100 * time.Millisecond, Log: "ok"}
	w := &Worker{Queue: fq, Runner: r, Reporter: nil, Cfg: Config{OutputDir: dir, CacheDir: dir, BatchSize: 10}}
	if err := w.LoadPlan(); err != nil {
		t.Fatal(err)
	}
	if err := w.Drain(context.Background()); err != nil {
		t.Fatalf("drain error: %v", err)
	}
	if got := len(r.Calls); got != 1 {
		t.Fatalf("expected 1 build from plan, got %d", got)
	}
	if r.Calls[0].Name != "pkg" {
		t.Fatalf("expected build job for pkg, got %s", r.Calls[0].Name)
	}
}
