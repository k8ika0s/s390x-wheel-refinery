package cas

import (
	"bytes"
	"context"
	"fmt"
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
	initURL := fmt.Sprintf("%s/v2/%s/blobs/uploads/", strings.TrimRight(p.BaseURL, "/"), repo)
	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, nil)
	if err != nil {
		return "", err
	}
	if p.Username != "" || p.Password != "" {
		initReq.SetBasicAuth(p.Username, p.Password)
	}
	initResp, err := p.client().Do(initReq)
	if err != nil {
		return "", err
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("init upload status %d", initResp.StatusCode)
	}
	loc := initResp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("upload location missing")
	}
	uploadURL := loc
	if strings.HasPrefix(loc, "/") {
		uploadURL = strings.TrimRight(p.BaseURL, "/") + loc
	}
	putURL := uploadURL
	if strings.Contains(uploadURL, "?") {
		putURL = uploadURL + "&digest=" + id.Digest
	} else {
		putURL = uploadURL + "?digest=" + id.Digest
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	if mediaType != "" {
		putReq.Header.Set("Content-Type", mediaType)
	}
	if p.Username != "" || p.Password != "" {
		putReq.SetBasicAuth(p.Username, p.Password)
	}
	putResp, err := p.client().Do(putReq)
	if err != nil {
		return "", err
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("push status %d", putResp.StatusCode)
	}
	return fmt.Sprintf("%s/v2/%s/blobs/%s", strings.TrimRight(p.BaseURL, "/"), repo, id.Digest), nil
}
