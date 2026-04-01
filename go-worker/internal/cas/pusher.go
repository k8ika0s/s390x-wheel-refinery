package cas

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

// Pusher uploads blobs to an OCI registry (Zot-compatible) under /v2/<repo>/blobs/uploads.
type Pusher struct {
	BaseURL  string
	Repo     string
	Username string
	Password string
	Client   *http.Client
}

func (p Pusher) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Push uploads content and returns a blob URL if successful.
func (p Pusher) Push(ctx context.Context, id artifact.ID, content []byte, mediaType string) (string, error) {
	if p.BaseURL == "" || id.Digest == "" {
		return "", fmt.Errorf("missing base URL or digest")
	}
	repo := strings.Trim(p.Repo, "/")
	if repo == "" {
		repo = "artifacts"
	}
	blobDigest := blobDigest(content)
	if err := p.pushBlob(ctx, repo, blobDigest, content, mediaType); err != nil {
		return "", err
	}

	ref := refForDigest(id.Digest)
	manifestPayload, configDigest, configPayload, err := buildManifestPayload(ref, id.Digest, blobDigest, int64(len(content)), mediaType)
	if err != nil {
		return "", err
	}
	if err := p.pushBlob(ctx, repo, configDigest, configPayload, ociConfigMediaType); err != nil {
		return "", err
	}

	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", strings.TrimRight(p.BaseURL, "/"), repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, manifestURL, bytes.NewReader(manifestPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", ociManifestMediaType)
	if p.Username != "" || p.Password != "" {
		req.SetBasicAuth(p.Username, p.Password)
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("manifest push status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return manifestURL, nil
}

func (p Pusher) pushBlob(ctx context.Context, repo, digest string, content []byte, mediaType string) error {
	initURL := fmt.Sprintf("%s/v2/%s/blobs/uploads/", strings.TrimRight(p.BaseURL, "/"), repo)
	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, nil)
	if err != nil {
		return err
	}
	if p.Username != "" || p.Password != "" {
		initReq.SetBasicAuth(p.Username, p.Password)
	}
	initResp, err := p.client().Do(initReq)
	if err != nil {
		return err
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(initResp.Body)
		return fmt.Errorf("init upload status %d: %s", initResp.StatusCode, strings.TrimSpace(string(body)))
	}
	loc := initResp.Header.Get("Location")
	if loc == "" {
		return fmt.Errorf("upload location missing")
	}
	uploadURL := loc
	if strings.HasPrefix(loc, "/") {
		uploadURL = strings.TrimRight(p.BaseURL, "/") + loc
	}
	putURL := uploadURL
	if strings.Contains(uploadURL, "?") {
		putURL = uploadURL + "&digest=" + digest
	} else {
		putURL = uploadURL + "?digest=" + digest
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(content))
	if err != nil {
		return err
	}
	if mediaType != "" {
		putReq.Header.Set("Content-Type", mediaType)
	}
	if p.Username != "" || p.Password != "" {
		putReq.SetBasicAuth(p.Username, p.Password)
	}
	putResp, err := p.client().Do(putReq)
	if err != nil {
		return err
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("push status %d: %s", putResp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
