package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
)

// fakeStore implements only what we need for plan tests.
type fakeStore struct {
	lastPlan []store.PlanNode
}

func (f *fakeStore) Recent(ctx context.Context, limit, offset int, pkg, status string) ([]store.Event, error) {
	return nil, nil
}
func (f *fakeStore) History(ctx context.Context, filter store.HistoryFilter) ([]store.Event, error) {
	return nil, nil
}
func (f *fakeStore) Summary(ctx context.Context, failureLimit int) (store.Summary, error) {
	return store.Summary{}, nil
}
func (f *fakeStore) PackageSummary(ctx context.Context, name string) (store.PackageSummary, error) {
	return store.PackageSummary{}, nil
}
func (f *fakeStore) LatestEvent(ctx context.Context, name, version string) (store.Event, error) {
	return store.Event{}, nil
}
func (f *fakeStore) Failures(ctx context.Context, name string, limit int) ([]store.Event, error) {
	return nil, nil
}
func (f *fakeStore) Variants(ctx context.Context, name string, limit int) ([]store.Event, error) {
	return nil, nil
}
func (f *fakeStore) TopFailures(ctx context.Context, limit int) ([]store.Stat, error) {
	return nil, nil
}
func (f *fakeStore) TopSlowest(ctx context.Context, limit int) ([]store.Stat, error) {
	return nil, nil
}
func (f *fakeStore) RecordEvent(ctx context.Context, evt store.Event) error {
	return nil
}
func (f *fakeStore) ListHints(ctx context.Context) ([]store.Hint, error) {
	return nil, nil
}
func (f *fakeStore) GetHint(ctx context.Context, id string) (store.Hint, error) {
	return store.Hint{}, nil
}
func (f *fakeStore) PutHint(ctx context.Context, hint store.Hint) error {
	return nil
}
func (f *fakeStore) DeleteHint(ctx context.Context, id string) error {
	return nil
}
func (f *fakeStore) GetLog(ctx context.Context, name, version string) (store.LogEntry, error) {
	return store.LogEntry{}, nil
}
func (f *fakeStore) SearchLogs(ctx context.Context, q string, limit int) ([]store.LogEntry, error) {
	return nil, nil
}
func (f *fakeStore) PutLog(ctx context.Context, entry store.LogEntry) error {
	return nil
}
func (f *fakeStore) Plan(ctx context.Context) ([]store.PlanNode, error) {
	return f.lastPlan, nil
}
func (f *fakeStore) SavePlan(ctx context.Context, runID string, nodes []store.PlanNode) error {
	f.lastPlan = nodes
	return nil
}
func (f *fakeStore) Manifest(ctx context.Context, limit int) ([]store.ManifestEntry, error) {
	return nil, nil
}
func (f *fakeStore) SaveManifest(ctx context.Context, entries []store.ManifestEntry) error {
	return nil
}
func (f *fakeStore) Artifacts(ctx context.Context, limit int) ([]store.Artifact, error) {
	return nil, nil
}

// fakeQueue implements only Stats for these tests.
type fakeQueue struct {
	length int
}

func (f *fakeQueue) Enqueue(ctx context.Context, req queue.Request) error { return nil }
func (f *fakeQueue) List(ctx context.Context) ([]queue.Request, error)    { return nil, nil }
func (f *fakeQueue) Clear(ctx context.Context) error                      { return nil }
func (f *fakeQueue) Stats(ctx context.Context) (queue.Stats, error) {
	return queue.Stats{Length: f.length}, nil
}
func (f *fakeQueue) Pop(ctx context.Context, max int) ([]queue.Request, error) { return nil, nil }

func TestPlanComputeProxiesToWorker(t *testing.T) {
	// fake worker server that returns a plan snapshot
	workerPlan := []byte(`{"run_id":"w1","plan":[{"name":"pkg","version":"1.0","python_tag":"cp311","platform_tag":"manylinux2014_s390x","action":"build"}]}`)
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(workerPlan)
	}))
	defer worker.Close()

	fs := &fakeStore{}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, Config: config.Config{WorkerPlanURL: worker.URL, WorkerToken: ""}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/plan/compute", "application/json", nil)
	if err != nil {
		t.Fatalf("post plan compute: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	var snap map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// saved to store?
	if len(fs.lastPlan) == 0 {
		t.Fatalf("plan not saved to store")
	}
}

func TestPlanPostSavesPlan(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := bytes.NewBufferString(`{"run_id":"r1","plan":[{"name":"pkg","version":"1.0","python_tag":"cp311","platform_tag":"manylinux2014_s390x","action":"build"}]}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/plan", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if len(fs.lastPlan) != 1 {
		t.Fatalf("plan not saved")
	}
}
