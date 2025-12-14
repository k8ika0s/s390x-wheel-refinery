package cas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

func TestZotStoreHas(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		switch r.URL.Path {
		case "/v2/artifacts/manifests/sha256:present":
			w.WriteHeader(http.StatusOK)
		case "/v2/artifacts/manifests/sha256:missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	store := ZotStore{BaseURL: ts.URL}
	ctx := context.Background()
	idPresent := artifact.ID{Type: artifact.WheelType, Digest: "sha256:present"}
	idMissing := artifact.ID{Type: artifact.WheelType, Digest: "sha256:missing"}

	ok, err := store.Has(ctx, idPresent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected present to be true")
	}
	if gotPath != "/v2/artifacts/manifests/sha256:present" {
		t.Fatalf("unexpected path: %s", gotPath)
	}

	ok, err = store.Has(ctx, idMissing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected missing to be false")
	}
}
