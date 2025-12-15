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

func TestFetcherFetchesBlob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/artifacts/blobs/sha256:test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("blobdata"))
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
	if string(data) != "blobdata" {
		t.Fatalf("unexpected contents: %s", string(data))
	}
}
