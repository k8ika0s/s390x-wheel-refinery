package cas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

// Fetcher downloads artifacts from an OCI registry (Zot-compatible) to a local path.
type Fetcher struct {
	BaseURL  string
	Repo     string
	Username string
	Password string
	Client   *http.Client
}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// Fetch downloads the blob for the given artifact digest into destPath.
// Assumes registry supports /v2/<repo>/blobs/<digest>.
func (f Fetcher) Fetch(ctx context.Context, id artifact.ID, destPath string) error {
	if f.BaseURL == "" || id.Digest == "" {
		return fmt.Errorf("missing base URL or digest")
	}
	repo := strings.Trim(f.Repo, "/")
	if repo == "" {
		repo = "artifacts"
	}
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", strings.TrimRight(f.BaseURL, "/"), repo, id.Digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if f.Username != "" || f.Password != "" {
		req.SetBasicAuth(f.Username, f.Password)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: unexpected status %d", id.Digest, resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
