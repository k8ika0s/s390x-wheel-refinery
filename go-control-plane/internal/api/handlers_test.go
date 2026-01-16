package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/settings"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
)

// fakeStore implements only what we need for plan tests.
type fakeStore struct {
	lastPlan        []store.PlanNode
	lastEvent       store.Event
	nextPendingID   int64
	listPending     []store.PendingInput
	lastPending     store.PendingInput
	pendingStatuses []struct {
		id     int64
		status string
		errMsg string
	}
	restoredPendingID int64
	queuedBuilds      []store.PlanNode
	leaseBuildsMax    int
	leaseBuilds       []store.BuildStatus
	requeueCalls      int
	requeueLeaseAge   int
	requeueBuildAge   int
	requeueResp       []store.BuildStatus
	lastBuildUpdatePlanID int64
	lastBuildUpdateNodeID string
	lastLogChunksAfterID  int64
	lastLogChunksAfterSeq int64
	lastLogChunksAttempt  int
	lastLogChunksLimit    int
	manifestPackages      []string
	manifestByNormalized  map[string][]store.ManifestEntry
	trimEventsCalls       int
	trimAttemptsCalls     int
	trimManifestsCalls    int
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
	f.lastEvent = evt
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
func (f *fakeStore) PutLogChunk(ctx context.Context, chunk store.LogChunk) (int64, error) {
	return 0, nil
}
func (f *fakeStore) ListLogChunks(ctx context.Context, name, version string, afterID int64, afterSeq int64, attempt int, limit int) ([]store.LogChunk, error) {
	f.lastLogChunksAfterID = afterID
	f.lastLogChunksAfterSeq = afterSeq
	f.lastLogChunksAttempt = attempt
	f.lastLogChunksLimit = limit
	return nil, nil
}
func (f *fakeStore) TailLogChunks(ctx context.Context, name, version string, attempt int, limit int) ([]store.LogChunk, error) {
	f.lastLogChunksAttempt = attempt
	f.lastLogChunksLimit = limit
	return nil, nil
}
func (f *fakeStore) TrimLogChunks(ctx context.Context, name, version string, max int) (int64, error) {
	return 0, nil
}
func (f *fakeStore) TrimLogChunksBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeStore) TrimLogsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeStore) TrimEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	f.trimEventsCalls++
	return 0, nil
}
func (f *fakeStore) TrimBuildAttemptsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	f.trimAttemptsCalls++
	return 0, nil
}
func (f *fakeStore) TrimManifestsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	f.trimManifestsCalls++
	return 0, nil
}
func (f *fakeStore) Plan(ctx context.Context) ([]store.PlanNode, error) {
	return f.lastPlan, nil
}
func (f *fakeStore) PlanSnapshot(ctx context.Context, planID int64) (store.PlanSnapshot, error) {
	return store.PlanSnapshot{ID: planID, RunID: "test", Plan: f.lastPlan}, nil
}
func (f *fakeStore) LatestPlanSnapshot(ctx context.Context) (store.PlanSnapshot, error) {
	return store.PlanSnapshot{ID: 1, RunID: "latest", Plan: f.lastPlan}, nil
}
func (f *fakeStore) ListPlans(ctx context.Context, limit int) ([]store.PlanSummary, error) {
	return nil, nil
}
func (f *fakeStore) SavePlan(ctx context.Context, runID string, nodes []store.PlanNode, dag json.RawMessage) (int64, error) {
	f.lastPlan = nodes
	return 1, nil
}
func (f *fakeStore) DeletePlans(ctx context.Context, planID int64) (int64, error) {
	return 0, nil
}
func (f *fakeStore) QueueBuildsFromPlan(ctx context.Context, runID string, planID int64, nodes []store.PlanNode) error {
	f.queuedBuilds = append(f.queuedBuilds, nodes...)
	return nil
}
func (f *fakeStore) Manifest(ctx context.Context, limit int) ([]store.ManifestEntry, error) {
	return nil, nil
}
func (f *fakeStore) ManifestPackages(ctx context.Context, limit int) ([]string, error) {
	return f.manifestPackages, nil
}
func (f *fakeStore) ManifestByNormalizedName(ctx context.Context, normalized string, limit int) ([]store.ManifestEntry, error) {
	if f.manifestByNormalized == nil {
		return nil, nil
	}
	return f.manifestByNormalized[normalized], nil
}
func (f *fakeStore) SaveManifest(ctx context.Context, entries []store.ManifestEntry) error {
	return nil
}
func (f *fakeStore) Artifacts(ctx context.Context, limit int) ([]store.Artifact, error) {
	return nil, nil
}
func (f *fakeStore) AddPendingInput(ctx context.Context, pi store.PendingInput) (int64, error) {
	if f.nextPendingID == 0 {
		f.nextPendingID = 1
	}
	f.lastPending = pi
	id := f.nextPendingID
	f.nextPendingID++
	return id, nil
}
func (f *fakeStore) ListPendingInputs(ctx context.Context, status string) ([]store.PendingInput, error) {
	return f.listPending, nil
}
func (f *fakeStore) PendingInputCount(ctx context.Context, status string) (int, error) {
	return len(f.listPending), nil
}
func (f *fakeStore) UpdatePendingInputStatus(ctx context.Context, id int64, status, errMsg string) error {
	f.pendingStatuses = append(f.pendingStatuses, struct {
		id     int64
		status string
		errMsg string
	}{id: id, status: status, errMsg: errMsg})
	return nil
}
func (f *fakeStore) DeletePendingInput(ctx context.Context, id int64) (store.PendingInput, error) {
	return store.PendingInput{ID: id}, nil
}
func (f *fakeStore) RestorePendingInput(ctx context.Context, id int64) (store.PendingInput, error) {
	f.restoredPendingID = id
	return store.PendingInput{ID: id, Status: "pending"}, nil
}
func (f *fakeStore) LinkPlanToPendingInput(ctx context.Context, pendingID, planID int64) error {
	return nil
}
func (f *fakeStore) UpdatePendingInputsForPlan(ctx context.Context, planID int64, status string) (int64, error) {
	return 0, nil
}
func (f *fakeStore) ListBuilds(ctx context.Context, status string, limit int, planID int64, pkg string, version string) ([]store.BuildStatus, error) {
	return nil, nil
}
func (f *fakeStore) BuildQueueStats(ctx context.Context) (store.BuildQueueStats, error) {
	return store.BuildQueueStats{}, nil
}
func (f *fakeStore) UpdateBuildStatus(ctx context.Context, pkg, version, status, errMsg, summary string, attempts int, backoffUntil int64, backoffReason string, backoffSeconds int, reasonCode string, reasonDetail string, recipes []string, hintIDs []string, planID int64, nodeID string) error {
	f.lastBuildUpdatePlanID = planID
	f.lastBuildUpdateNodeID = nodeID
	return nil
}
func (f *fakeStore) UpsertBuildAttempt(ctx context.Context, attempt store.BuildAttempt) error {
	return nil
}
func (f *fakeStore) ListBuildAttempts(ctx context.Context, pkg, version string, limit int) ([]store.BuildAttempt, error) {
	return nil, nil
}
func (f *fakeStore) LeaseBuilds(ctx context.Context, max int) ([]store.BuildStatus, error) {
	f.leaseBuildsMax = max
	return f.leaseBuilds, nil
}
func (f *fakeStore) RequeueStaleBuilds(ctx context.Context, leaseAgeSec int, buildAgeSec int) ([]store.BuildStatus, error) {
	f.requeueCalls++
	f.requeueLeaseAge = leaseAgeSec
	f.requeueBuildAge = buildAgeSec
	return f.requeueResp, nil
}
func (f *fakeStore) DeleteBuilds(ctx context.Context, status string) (int64, error) {
	return 0, nil
}
func (f *fakeStore) UpsertWorkerStatus(ctx context.Context, status store.WorkerStatus) error {
	return nil
}
func (f *fakeStore) ListWorkers(ctx context.Context) ([]store.WorkerStatus, error) {
	return nil, nil
}
func (f *fakeStore) GetSettings(ctx context.Context) (settings.Settings, error) {
	return settings.ApplyDefaults(settings.Settings{}), nil
}
func (f *fakeStore) SaveSettings(ctx context.Context, s settings.Settings) error {
	return nil
}

// fakeQueue implements only Stats for these tests.
type fakeQueue struct {
	length int
	enq    []queue.Request
}

func (f *fakeQueue) Enqueue(ctx context.Context, req queue.Request) error {
	f.enq = append(f.enq, req)
	return nil
}
func (f *fakeQueue) List(ctx context.Context) ([]queue.Request, error) { return nil, nil }
func (f *fakeQueue) Clear(ctx context.Context) error                   { return nil }
func (f *fakeQueue) Stats(ctx context.Context) (queue.Stats, error) {
	return queue.Stats{Length: f.length}, nil
}
func (f *fakeQueue) Pop(ctx context.Context, max int) ([]queue.Request, error) { return nil, nil }

type fakePlanQueue struct {
	ids []string
	err error
	pop []string
}

func (f *fakePlanQueue) Enqueue(ctx context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.ids = append(f.ids, id)
	return nil
}

func (f *fakePlanQueue) Pop(ctx context.Context, max int) ([]string, error) {
	return f.pop, f.err
}
func (f *fakePlanQueue) Len(ctx context.Context) (int64, error) {
	return int64(len(f.ids) + len(f.pop)), f.err
}
func (f *fakePlanQueue) Clear(ctx context.Context) ([]string, error) {
	return f.ids, f.err
}

type fakeObjectStore struct {
	lastKey         string
	lastContentType string
	lastData        []byte
}

func (f *fakeObjectStore) Put(_ context.Context, key string, data []byte, contentType string) error {
	f.lastKey = key
	f.lastContentType = contentType
	f.lastData = append([]byte(nil), data...)
	return nil
}

func (f *fakeObjectStore) URL(key string) string {
	return "http://example/" + key
}

func mustMultipart(t *testing.T, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

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
	fq := &fakeQueue{}
	h := &Handler{Store: fs, Queue: fq, Config: config.Config{WorkerPlanURL: worker.URL, WorkerToken: "", AutoBuild: true}}
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
	if len(fs.queuedBuilds) != 1 || fs.queuedBuilds[0].Name != "pkg" {
		t.Fatalf("expected enqueue from plan compute, got %+v", fs.queuedBuilds)
	}
}

func TestPlanPostSavesPlan(t *testing.T) {
	fs := &fakeStore{}
	fq := &fakeQueue{}
	h := &Handler{Store: fs, Queue: fq, Config: config.Config{AutoBuild: true}}
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
	if len(fs.queuedBuilds) != 1 || fs.queuedBuilds[0].Name != "pkg" {
		t.Fatalf("expected enqueue from plan, got %+v", fs.queuedBuilds)
	}
}

func TestHistoryPostRecordsEvent(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := bytes.NewBufferString(`{"name":"pkg","version":"1.0","status":"built","python_tag":"cp311","platform_tag":"manylinux2014_s390x"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/history", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if fs.lastEvent.Name != "pkg" || fs.lastEvent.Version != "1.0" || fs.lastEvent.Status != "built" {
		t.Fatalf("event not recorded: %+v", fs.lastEvent)
	}
	if fs.lastEvent.Timestamp == 0 {
		t.Fatalf("timestamp not set")
	}
}

func TestPendingInputsList(t *testing.T) {
	fs := &fakeStore{listPending: []store.PendingInput{{ID: 1, Filename: "requirements-123.txt", Status: "pending"}}}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/pending-inputs")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out []store.PendingInput
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].Filename != "requirements-123.txt" {
		t.Fatalf("unexpected list: %+v", out)
	}
}

func TestPendingInputManualEnqueue(t *testing.T) {
	fs := &fakeStore{}
	pq := &fakePlanQueue{}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, PlanQ: pq, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/pending-inputs/5/enqueue-plan", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if len(pq.ids) != 1 || pq.ids[0] != "5" {
		t.Fatalf("expected enqueue id 5, got %+v", pq.ids)
	}
	if len(fs.pendingStatuses) == 0 || fs.pendingStatuses[0].status != "planning" {
		t.Fatalf("expected status update planning, got %+v", fs.pendingStatuses)
	}
}

func TestPendingInputPop(t *testing.T) {
	fs := &fakeStore{}
	pq := &fakePlanQueue{pop: []string{"7", "8"}}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, PlanQ: pq, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/pending-inputs/pop?max=2", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	idsAny, ok := out["ids"].([]any)
	if !ok || len(idsAny) != 2 {
		t.Fatalf("unexpected ids: %+v", out)
	}
	if len(fs.pendingStatuses) != 2 || fs.pendingStatuses[0].status != "planning" || fs.pendingStatuses[1].status != "planning" {
		t.Fatalf("expected planning status updates, got %+v", fs.pendingStatuses)
	}
}

func TestPendingInputStatusUpdate(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := bytes.NewBufferString(`{"status":"planned","error":""}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/pending-inputs/status/9", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if len(fs.pendingStatuses) != 1 || fs.pendingStatuses[0].id != 9 || fs.pendingStatuses[0].status != "planned" {
		t.Fatalf("expected planned update, got %+v", fs.pendingStatuses)
	}
}

func TestPendingInputRestore(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{Store: fs, Queue: &fakeQueue{}, Config: config.Config{}}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/pending-inputs/12/restore", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if fs.restoredPendingID != 12 {
		t.Fatalf("expected restore id 12, got %d", fs.restoredPendingID)
	}
}

func TestRequirementsUploadAutoEnqueue(t *testing.T) {
	fs := &fakeStore{nextPendingID: 42}
	pq := &fakePlanQueue{}
	fo := &fakeObjectStore{}
	h := &Handler{
		Store: fs, Queue: &fakeQueue{}, PlanQ: pq, InputStore: fo,
		Config: config.Config{
			AutoPlan:            true,
			ObjectStoreEndpoint: "minio:9000",
			ObjectStoreBucket:   "inputs",
			InputObjectPrefix:   "inputs",
		},
	}
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body, contentType := mustMultipart(t, "requirements.txt", "pkg==1.0\n")
	resp, err := http.Post(ts.URL+"/api/requirements/upload", contentType, body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pending, ok := out["pending_id"].(float64); !ok || int64(pending) != 42 {
		t.Fatalf("expected pending_id 42, got %v", out["pending_id"])
	}
	if len(pq.ids) != 1 || pq.ids[0] != "42" {
		t.Fatalf("expected enqueue id 42, got %+v", pq.ids)
	}
	if len(fs.pendingStatuses) == 0 || fs.pendingStatuses[0].status != "planning" {
		t.Fatalf("expected planning status update, got %+v", fs.pendingStatuses)
	}
	if fo.lastKey == "" || len(fo.lastData) == 0 {
		t.Fatalf("expected object store upload, got key=%q bytes=%d", fo.lastKey, len(fo.lastData))
	}
	if fs.lastPending.SourceType != "requirements" {
		t.Fatalf("expected requirements source_type, got %q", fs.lastPending.SourceType)
	}
}

func TestBuildQueuePopRequeuesStale(t *testing.T) {
	fs := &fakeStore{
		leaseBuilds: []store.BuildStatus{
			{Package: "pkg", Version: "1.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Attempts: 2, NodeID: "node-1"},
		},
	}
	h := &Handler{
		Store: fs,
		Config: config.Config{
			WorkerToken:       "token",
			BuildLeaseTimeout: 120,
			BuildStallTimeout: 300,
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/build-queue/pop?max=7", nil)
	req.Header.Set("X-Worker-Token", "token")
	rec := httptest.NewRecorder()
	h.buildQueuePop(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fs.requeueCalls != 1 {
		t.Fatalf("expected requeue call, got %d", fs.requeueCalls)
	}
	if fs.requeueLeaseAge != 120 || fs.requeueBuildAge != 300 {
		t.Fatalf("unexpected requeue args: lease=%d build=%d", fs.requeueLeaseAge, fs.requeueBuildAge)
	}
	if fs.leaseBuildsMax != 7 {
		t.Fatalf("expected lease max 7, got %d", fs.leaseBuildsMax)
	}

	var payload struct {
		Builds []map[string]any `json:"builds"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(payload.Builds))
	}
	if payload.Builds[0]["node_id"] != "node-1" {
		t.Fatalf("expected node_id node-1, got %v", payload.Builds[0]["node_id"])
	}
}

func TestBuildStatusUpdateCapturesNodeID(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{
		Store: fs,
		Config: config.Config{
			WorkerToken: "token",
		},
	}
	body := map[string]any{
		"package":  "demo",
		"version":  "1.0.0",
		"status":   "building",
		"attempts": 1,
		"plan_id":  12,
		"node_id":  "node-abc",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/builds/status", bytes.NewReader(payload))
	req.Header.Set("X-Worker-Token", "token")
	rec := httptest.NewRecorder()
	h.buildStatusUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fs.lastBuildUpdatePlanID != 12 {
		t.Fatalf("expected plan_id 12, got %d", fs.lastBuildUpdatePlanID)
	}
	if fs.lastBuildUpdateNodeID != "node-abc" {
		t.Fatalf("expected node_id node-abc, got %q", fs.lastBuildUpdateNodeID)
	}
	if fs.lastEvent.Metadata == nil {
		t.Fatalf("expected event metadata")
	}
	if fs.lastEvent.Metadata["node_id"] != "node-abc" {
		t.Fatalf("expected event node_id node-abc, got %v", fs.lastEvent.Metadata["node_id"])
	}
	if got := fs.lastEvent.Metadata["plan_id"]; got != int64(12) && got != float64(12) && got != int(12) {
		t.Fatalf("expected event plan_id 12, got %v", got)
	}
}

func TestLogsChunksHonorsAttemptAndSeq(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{Store: fs}
	req := httptest.NewRequest(http.MethodGet, "/api/logs/chunks/demo/1.0?attempt=2&after_seq=10&limit=12", nil)
	rec := httptest.NewRecorder()
	h.logsChunks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fs.lastLogChunksAttempt != 2 {
		t.Fatalf("expected attempt 2, got %d", fs.lastLogChunksAttempt)
	}
	if fs.lastLogChunksAfterSeq != 10 {
		t.Fatalf("expected after_seq 10, got %d", fs.lastLogChunksAfterSeq)
	}
	if fs.lastLogChunksLimit != 12 {
		t.Fatalf("expected limit 12, got %d", fs.lastLogChunksLimit)
	}
}

func TestSimpleIndexRoot(t *testing.T) {
	fs := &fakeStore{
		manifestPackages: []string{"NumPy", "requests"},
	}
	h := &Handler{Store: fs}
	req := httptest.NewRequest(http.MethodGet, "/simple", nil)
	rec := httptest.NewRecorder()
	h.simpleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/simple/numpy/") || !strings.Contains(body, "/simple/requests/") {
		t.Fatalf("expected simple index entries, got %s", body)
	}
}

func TestSimpleIndexPackage(t *testing.T) {
	fs := &fakeStore{
		manifestByNormalized: map[string][]store.ManifestEntry{
			"numpy": {
				{Name: "NumPy", Version: "1.0.0", WheelURL: "https://example.com/numpy-1.0.0.whl"},
			},
		},
	}
	h := &Handler{Store: fs}
	req := httptest.NewRequest(http.MethodGet, "/simple/numpy/", nil)
	rec := httptest.NewRecorder()
	h.simpleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "numpy-1.0.0.whl") || !strings.Contains(body, "https://example.com/numpy-1.0.0.whl") {
		t.Fatalf("expected wheel link, got %s", body)
	}
}

func TestMaybeSweepRetention(t *testing.T) {
	fs := &fakeStore{}
	h := &Handler{
		Store: fs,
		Config: config.Config{
			EventRetentionDays:    1,
			AttemptRetentionDays:  1,
			ManifestRetentionDays: 1,
			LogRetentionSweepSec:  1,
		},
	}
	h.maybeSweepLogs(context.Background())
	if fs.trimEventsCalls != 1 {
		t.Fatalf("expected events trim call, got %d", fs.trimEventsCalls)
	}
	if fs.trimAttemptsCalls != 1 {
		t.Fatalf("expected attempts trim call, got %d", fs.trimAttemptsCalls)
	}
	if fs.trimManifestsCalls != 1 {
		t.Fatalf("expected manifests trim call, got %d", fs.trimManifestsCalls)
	}
}
