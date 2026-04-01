package cas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

func TestFetcherFetchesArtifactLayer(t *testing.T) {
	layerContent := []byte("blobdata")
	layerDigest := blobDigest(layerContent)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/artifacts/manifests/sha256-test":
			payload, _, _, err := buildManifestPayload("sha256-test", "sha256:test", layerDigest, int64(len(layerContent)), "application/octet-stream")
			if err != nil {
				t.Fatalf("build manifest: %v", err)
			}
			w.Header().Set("Content-Type", ociManifestMediaType)
			_, _ = w.Write(payload)
		case "/v2/artifacts/blobs/" + layerDigest:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(layerContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	f := Fetcher{BaseURL: ts.URL}
	dest := filepath.Join(t.TempDir(), "blob.bin")
	id := artifact.ID{Type: artifact.WheelType, Digest: "sha256:test"}
	if err := f.Fetch(context.Background(), id, dest); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != string(layerContent) {
		t.Fatalf("unexpected contents: %s", string(data))
	}
}
