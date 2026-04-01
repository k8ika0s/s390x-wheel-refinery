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

// Fetch resolves the artifact manifest and downloads the first layer blob into destPath.
func (f Fetcher) Fetch(ctx context.Context, id artifact.ID, destPath string) error {
	if f.BaseURL == "" || id.Digest == "" {
		return fmt.Errorf("missing base URL or digest")
	}
	repo := strings.Trim(f.Repo, "/")
	if repo == "" {
		repo = "artifacts"
	}
	ref := refForDigest(id.Digest)
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", strings.TrimRight(f.BaseURL, "/"), repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", ociManifestMediaType)
	if f.Username != "" || f.Password != "" {
		req.SetBasicAuth(f.Username, f.Password)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch manifest %s: unexpected status %d", id.Digest, resp.StatusCode)
	}
	manifestPayload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	manifest, err := parseManifest(manifestPayload)
	if err != nil {
		return err
	}
	layerDigest := manifest.Layers[0].Digest
	if layerDigest == "" {
		return fmt.Errorf("manifest missing layer digest for %s", id.Digest)
	}

	blobURL := fmt.Sprintf("%s/v2/%s/blobs/%s", strings.TrimRight(f.BaseURL, "/"), repo, layerDigest)
	blobReq, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL, nil)
	if err != nil {
		return err
	}
	if f.Username != "" || f.Password != "" {
		blobReq.SetBasicAuth(f.Username, f.Password)
	}
	blobResp, err := f.client().Do(blobReq)
	if err != nil {
		return err
	}
	defer blobResp.Body.Close()
	if blobResp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch blob %s: unexpected status %d", layerDigest, blobResp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, blobResp.Body); err != nil {
		return err
	}
	if ok, err := verifyFetchedBlob(destPath, layerDigest); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("fetched blob digest mismatch: expected %s", layerDigest)
	}
	return nil
}

func verifyFetchedBlob(path, expected string) (bool, error) {
	if expected == "" {
		return false, fmt.Errorf("expected digest missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return blobDigest(data) == expected, nil
}
