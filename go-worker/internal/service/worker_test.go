package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	}
	job := runner.Job{WheelDigest: "sha256:abc", WheelAction: "reuse"}
	if err := w.fetchArtifact(context.Background(), job); err != nil {
		t.Fatalf("fetchArtifact: %v", err)
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
			{ID: artifact.ID{Type: artifact.WheelType, Digest: digest}, Type: plan.NodeWheel, Action: "reuse", Metadata: map[string]any{"name": "demo", "version": "1.0.0", "python_tag": "cp311", "platform_tag": "manylinux2014_s390x"}},
		},
	}
	w := &Worker{Cfg: Config{}, planSnap: snap}
	reqs := []queue.Request{{Package: "demo", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"}}
	jobs := w.match(reqs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].WheelDigest != digest {
		t.Fatalf("wheel digest not propagated: %s", jobs[0].WheelDigest)
	}
	if jobs[0].WheelAction != "reuse" {
		t.Fatalf("wheel action not propagated: %s", jobs[0].WheelAction)
	}
}
