package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
