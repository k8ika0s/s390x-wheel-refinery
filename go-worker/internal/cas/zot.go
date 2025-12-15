package cas

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

// ZotStore checks an OCI registry (e.g., Zot) for artifacts.
// It uses HEAD requests against /v2/<repo>/manifests/<digest>.
type ZotStore struct {
	BaseURL  string
	Repo     string
	Username string
	Password string
	Client   *http.Client
}

func (z ZotStore) client() *http.Client {
	if z.Client != nil {
		return z.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Has reports whether the digest exists as a manifest in the configured repo.
func (z ZotStore) Has(ctx context.Context, id artifact.ID) (bool, error) {
	if z.BaseURL == "" || id.Digest == "" {
		return false, nil
	}
	repo := strings.Trim(z.Repo, "/")
	if repo == "" {
		repo = "artifacts"
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", strings.TrimRight(z.BaseURL, "/"), repo, id.Digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	if z.Username != "" || z.Password != "" {
		req.SetBasicAuth(z.Username, z.Password)
	}
	resp, err := z.client().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("zot store unexpected status %d for %s", resp.StatusCode, url)
	}
}
