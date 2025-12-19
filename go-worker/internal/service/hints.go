package service

import (
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
