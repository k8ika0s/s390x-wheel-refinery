package cas

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

func TestPusherPushesBlob(t *testing.T) {
	var initCalled, putCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/artifacts/blobs/uploads/"):
			initCalled = true
			w.Header().Set("Location", "/v2/artifacts/blobs/uploads/uuid")
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v2/artifacts/blobs/uploads/uuid"):
			putCalled = true
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "bad", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	p := Pusher{BaseURL: ts.URL}
	url, err := p.Push(context.Background(), artifact.ID{Type: artifact.WheelType, Digest: "sha256:dead"}, []byte("data"), "application/octet-stream")
	if err != nil {
		body, _ := io.ReadAll(strings.NewReader(err.Error()))
		t.Fatalf("push failed: %v (%s)", err, string(body))
	}
	if !initCalled || !putCalled {
		t.Fatalf("upload flow not completed init=%v put=%v", initCalled, putCalled)
	}
	if url == "" {
		t.Fatalf("expected returned blob url")
	}
}
