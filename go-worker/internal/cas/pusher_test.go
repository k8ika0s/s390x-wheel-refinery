package cas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

func TestPusherPushesArtifactManifest(t *testing.T) {
	var initCalled, blobPutCalled, configPutCalled, manifestPutCalled bool
	layerDigest := blobDigest([]byte("data"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/artifacts/blobs/uploads/"):
			initCalled = true
			w.Header().Set("Location", "/v2/artifacts/blobs/uploads/uuid"+r.URL.RawQuery)
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v2/artifacts/blobs/uploads/uuid"):
			switch {
			case strings.Contains(r.URL.RawQuery, "digest="+layerDigest):
				blobPutCalled = true
			case strings.Contains(r.URL.RawQuery, "digest="):
				configPutCalled = true
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/v2/artifacts/manifests/sha256-dead":
			manifestPutCalled = true
			var manifest ociManifest
			if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			if len(manifest.Layers) != 1 || manifest.Layers[0].Digest != layerDigest {
				t.Fatalf("unexpected manifest layers: %+v", manifest.Layers)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "bad", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	p := Pusher{BaseURL: ts.URL}
	url, err := p.Push(context.Background(), artifact.ID{Type: artifact.WheelType, Digest: "sha256:dead"}, []byte("data"), "application/octet-stream")
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}
	if !initCalled || !blobPutCalled || !configPutCalled || !manifestPutCalled {
		t.Fatalf("upload flow not completed init=%v blob=%v config=%v manifest=%v", initCalled, blobPutCalled, configPutCalled, manifestPutCalled)
	}
	if url != ts.URL+"/v2/artifacts/manifests/sha256-dead" {
		t.Fatalf("unexpected returned url: %s", url)
	}
}
