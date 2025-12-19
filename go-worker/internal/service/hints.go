package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

func fetchHints(ctx context.Context, client *http.Client, cfg Config) ([]plan.Hint, error) {
	if cfg.ControlPlaneURL == "" {
		return nil, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	base := strings.TrimRight(cfg.ControlPlaneURL, "/")
	limit := 500
	offset := 0
	var out []plan.Hint
	for {
		url := fmt.Sprintf("%s/api/hints?limit=%d&offset=%d", base, limit, offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return out, err
		}
		if cfg.ControlPlaneToken != "" {
			req.Header.Set("X-Worker-Token", cfg.ControlPlaneToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			return out, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return out, fmt.Errorf("fetch hints status %d", resp.StatusCode)
		}
		var page []plan.Hint
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return out, err
		}
		resp.Body.Close()
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		if len(page) < limit {
			break
		}
		offset += len(page)
	}
	return out, nil
}

func upsertHint(ctx context.Context, client *http.Client, cfg Config, hint plan.Hint) error {
	if cfg.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL not set")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	base := strings.TrimRight(cfg.ControlPlaneURL, "/")
	url := fmt.Sprintf("%s/api/hints", base)
	data, err := json.Marshal(hint)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.ControlPlaneToken != "" {
		req.Header.Set("X-Worker-Token", cfg.ControlPlaneToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upsert hint status %d", resp.StatusCode)
	}
	return nil
}
