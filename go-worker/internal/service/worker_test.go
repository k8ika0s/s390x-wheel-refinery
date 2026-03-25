package service

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/cas"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

type fakeStore struct {
	keys []string
}

func (f *fakeStore) Put(_ context.Context, key string, _ []byte, _ string) error {
	f.keys = append(f.keys, key)
	return nil
}

func (f *fakeStore) Get(_ context.Context, _ string) ([]byte, string, error) {
	return nil, "", io.EOF
}

func (f *fakeStore) URL(key string) string {
	return "http://minio/" + key
}

func TestUploadArtifactsFiltersWheels(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	// matching wheel
	if err := os.WriteFile(filepath.Join(output, "demo-1.0.0-py3-none-any.whl"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-matching wheel
	if err := os.WriteFile(filepath.Join(output, "other-1.0.0-py3-none-any.whl"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{}
	w := &Worker{
		Cfg:   Config{OutputDir: output},
		Store: fs,
	}
	w.uploadArtifacts(context.Background(), runner.Job{Name: "demo", Version: "1.0.0"})
	if len(fs.keys) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(fs.keys))
	}
	if fs.keys[0] != "demo/1.0.0/demo-1.0.0-py3-none-any.whl" {
		t.Fatalf("unexpected key: %s", fs.keys[0])
	}
}

func TestFetchArtifactUsesFetcher(t *testing.T) {
	dir := t.TempDir()
	fetched := false
	w := &Worker{
		Cfg: Config{CacheDir: dir, LocalCASDir: filepath.Join(dir, "cas")},
		Fetcher: cas.Fetcher{
			BaseURL: "http://example",
			Repo:    "artifacts",
			Client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					fetched = true
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("data")),
						Header:     make(http.Header),
					}, nil
				}),
			},
		},
		packPath: make(map[string]string),
	}
	job := runner.Job{WheelDigest: "sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7", WheelAction: "reuse"}
	if err := w.fetchWheel(context.Background(), job); err != nil {
		t.Fatalf("fetchWheel: %v", err)
	}
	if !fetched {
		t.Fatalf("fetcher not invoked")
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMatchCarriesWheelDigestAndAction(t *testing.T) {
	digest := artifact.WheelKey{SourceDigest: "sha256:abc", PyTag: "cp311", PlatformTag: "manylinux2014_s390x", RuntimeDigest: "rt"}.Digest()
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{{Name: "demo", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}},
		DAG: []plan.DAGNode{
			{
				ID:       artifact.ID{Type: artifact.WheelType, Digest: digest},
				Type:     plan.NodeWheel,
				Action:   "reuse",
				Metadata: map[string]any{"name": "demo", "version": "1.0.0", "python_tag": "cp311", "platform_tag": "manylinux2014_s390x"},
				Inputs:   []artifact.ID{{Type: artifact.PackType, Digest: "sha256:pack1"}},
			},
		},
	}
	w := &Worker{Cfg: Config{}}
	reqs := []queue.Request{{Package: "demo", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"}}
	jobs := w.match(context.Background(), snap, reqs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].WheelDigest != digest {
		t.Fatalf("wheel digest not propagated: %s", jobs[0].WheelDigest)
	}
	if jobs[0].WheelAction != "reuse" {
		t.Fatalf("wheel action not propagated: %s", jobs[0].WheelAction)
	}
	if len(jobs[0].PackPaths) != 0 {
		t.Fatalf("pack paths should be empty without fetch")
	}
}

func TestCasURLHelper(t *testing.T) {
	w := &Worker{Cfg: Config{CASRegistryURL: "http://zot:5000", CASRegistryRepo: "artifacts"}}
	u := w.casURL(artifact.ID{Type: artifact.WheelType, Digest: "sha256:dead"})
	if u != "http://zot:5000/v2/artifacts/blobs/sha256:dead" {
		t.Fatalf("unexpected url: %s", u)
	}
}

type urlStore struct {
	url string
}

func (u urlStore) Put(_ context.Context, _ string, _ []byte, _ string) error { return nil }
func (u urlStore) Get(_ context.Context, _ string) ([]byte, string, error)   { return nil, "", io.EOF }
func (u urlStore) URL(_ string) string                                       { return u.url }

func TestObjectURLFallback(t *testing.T) {
	w := &Worker{Store: urlStore{url: "http://minio/bucket/path"}}
	u := w.objectURL(runner.Job{Name: "demo", Version: "1.0.0"}, "wheel")
	if u == "" {
		t.Fatalf("expected object url")
	}
}

func TestFetchRuntime(t *testing.T) {
	dir := t.TempDir()
	fetched := false
	tarBuf, rtDigest := sampleRuntimeTarWithDigest()
	w := &Worker{
		Cfg: Config{CacheDir: dir, LocalCASDir: filepath.Join(dir, "cas")},
		Fetcher: cas.Fetcher{
			BaseURL: "http://example",
			Repo:    "artifacts",
			Client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					fetched = true
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(tarBuf.Bytes())),
						Header:     make(http.Header),
					}, nil
				}),
			},
		},
		packPath: make(map[string]string),
	}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: rtDigest}
	path := w.fetchRuntime(context.Background(), "3.11", rtID, "reuse", nil)
	if path == "" {
		t.Fatalf("expected runtime path")
	}
	if !strings.HasSuffix(path, filepath.Join("usr", "local")) {
		t.Fatalf("expected prefix path, got %s", path)
	}
	if !fetched {
		t.Fatalf("fetcher not invoked for runtime")
	}
}

func TestResolvePacksBuildsArtifacts(t *testing.T) {
	dir := t.TempDir()
	w := &Worker{
		Cfg: Config{
			CacheDir:       dir,
			LocalCASDir:    filepath.Join(dir, "cas"),
			DefaultPackCmd: samplePackBuildCmd(),
		},
		packPath: make(map[string]string),
	}
	packID := artifact.ID{Type: artifact.PackType, Digest: "sha256:packstub"}
	paths := w.resolvePacks(context.Background(), []artifact.ID{packID}, map[string]string{packID.Digest: "build"}, map[string]map[string]any{packID.Digest: {"name": "stub"}})
	if len(paths) != 1 || paths[0] == "" {
		t.Fatalf("expected built pack path")
	}
	if _, err := os.Stat(filepath.Join(paths[0], "include", "stub.h")); err != nil {
		t.Fatalf("expected built pack payload: %v", err)
	}
}

func TestFetchRuntimeBuildsArtifacts(t *testing.T) {
	dir := t.TempDir()
	w := &Worker{Cfg: Config{
		CacheDir:          dir,
		LocalCASDir:       filepath.Join(dir, "cas"),
		DefaultRuntimeCmd: sampleRuntimeBuildCmd(),
	}}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: "sha256:rt-stub"}
	path := w.fetchRuntime(context.Background(), "3.11", rtID, "build", map[string]any{"note": "stub"})
	if path == "" {
		t.Fatalf("expected built runtime path")
	}
	if _, err := os.Stat(filepath.Join(path, "bin", "python3")); err != nil {
		t.Fatalf("expected runtime interpreter: %v", err)
	}
}

type stubQueue struct {
	reqs []queue.Request
}

func (s *stubQueue) Enqueue(ctx context.Context, req queue.Request) error { return nil }
func (s *stubQueue) List(ctx context.Context) ([]queue.Request, error)    { return nil, nil }
func (s *stubQueue) Clear(ctx context.Context) error                      { return nil }
func (s *stubQueue) Stats(ctx context.Context) (queue.Stats, error)       { return queue.Stats{}, nil }
func (s *stubQueue) Pop(ctx context.Context, max int) ([]queue.Request, error) {
	out := s.reqs
	s.reqs = nil
	return out, nil
}

type countingRunner struct {
	delay     time.Duration
	max       int32
	inFlight  int32
	totalRuns int32
}

func (r *countingRunner) Run(ctx context.Context, job runner.Job) (time.Duration, string, error) {
	cur := atomic.AddInt32(&r.inFlight, 1)
	defer atomic.AddInt32(&r.inFlight, -1)
	for {
		old := atomic.LoadInt32(&r.max)
		if cur <= old {
			break
		}
		if atomic.CompareAndSwapInt32(&r.max, old, cur) {
			break
		}
	}
	atomic.AddInt32(&r.totalRuns, 1)
	time.Sleep(r.delay)
	return r.delay, "", nil
}

func TestDrainRespectsBuildPoolSize(t *testing.T) {
	dir := t.TempDir()
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{
			{Name: "a", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"},
			{Name: "b", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"},
		},
	}
	if err := plan.Write(filepath.Join(dir, "plan.json"), snap); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	q := &stubQueue{reqs: []queue.Request{
		{Package: "a", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"},
		{Package: "b", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"},
	}}
	r := &countingRunner{delay: 50 * time.Millisecond}
	w := &Worker{
		Cfg: Config{
			OutputDir:     dir,
			CacheDir:      dir,
			BuildPoolSize: 1,
		},
		Queue:    q,
		Runner:   r,
		packPath: make(map[string]string),
	}
	if err := w.Drain(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if r.max > 1 {
		t.Fatalf("expected max concurrency 1, got %d", r.max)
	}
	if atomic.LoadInt32(&r.totalRuns) != 2 {
		t.Fatalf("expected 2 runs, got %d", r.totalRuns)
	}
}

func TestPopBuildQueueCapsMax(t *testing.T) {
	tests := []struct {
		name     string
		batch    int
		pool     int
		override int32
		wantMax  string
	}{
		{name: "pool overrides batch", batch: 10, pool: 4, wantMax: "4"},
		{name: "batch smaller than pool", batch: 2, pool: 5, wantMax: "2"},
		{name: "no batch uses pool", batch: 0, pool: 3, wantMax: "3"},
		{name: "override smaller than batch", batch: 10, pool: 5, override: 2, wantMax: "2"},
		{name: "no limits", batch: 0, pool: 0, wantMax: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMax := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMax = r.URL.Query().Get("max")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"builds":[]}`))
			}))
			defer srv.Close()

			worker := &Worker{
				Cfg: Config{
					ControlPlaneURL: srv.URL,
					BatchSize:       tt.batch,
					BuildPoolSize:   tt.pool,
				},
			}
			if tt.override > 0 {
				v := atomic.Int32{}
				v.Store(tt.override)
				worker.buildPoolSize = &v
			}

			if _, err := worker.popBuildQueue(context.Background()); err != nil {
				t.Fatalf("popBuildQueue: %v", err)
			}
			if gotMax != tt.wantMax {
				t.Fatalf("expected max=%q, got %q", tt.wantMax, gotMax)
			}
		})
	}
}

func sampleRuntimeTarWithDigest() (bytes.Buffer, string) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len("stub"))})
	_, _ = tw.Write([]byte("stub"))
	data := []byte("#!/bin/sh\nexit 0\n")
	_ = tw.WriteHeader(&tar.Header{Name: "usr/local/bin/python3", Mode: 0o755, Size: int64(len(data))})
	_, _ = tw.Write(data)
	_ = tw.Close()
	d := sha256.Sum256(buf.Bytes())
	return buf, "sha256:" + hex.EncodeToString(d[:])
}

func samplePackBuildCmd() string {
	return `mkdir -p "$PACK_OUTPUT/usr/local/include" && \
printf 'stub\n' > "$PACK_OUTPUT/usr/local/include/stub.h" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"stub","version":"1.0.0"}
EOF`
}

func sampleRuntimeBuildCmd() string {
	return `mkdir -p "$PACK_OUTPUT/usr/local/bin" && \
printf '#!/bin/sh\nexit 0\n' > "$PACK_OUTPUT/usr/local/bin/python3" && \
chmod +x "$PACK_OUTPUT/usr/local/bin/python3" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"cpython311","version":"3.11.0"}
EOF`
}
