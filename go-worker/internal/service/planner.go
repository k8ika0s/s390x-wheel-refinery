package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

type pendingInput struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

func plannerLoop(ctx context.Context, cfg Config, popURL, statusURL, listURL string) {
	interval := time.Duration(cfg.PlanPollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 15 * time.Second
	}
	client := &http.Client{Timeout: 30 * time.Second}
	batch := cfg.PlanPopBatch
	if batch <= 0 {
		batch = 5
	}
	inFlight := make(map[string]time.Time)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		now := time.Now()
		// expire in-flight markers older than 10 minutes
		for id, ts := range inFlight {
			if now.Sub(ts) > 10*time.Minute {
				delete(inFlight, id)
			}
		}
		ids, err := popPlanIDs(ctx, client, popURL, cfg.WorkerToken, batch)
		if err != nil {
			log.Printf("planner: pop error: %v", err)
			time.Sleep(interval)
			continue
		}
		if len(ids) == 0 {
			time.Sleep(interval)
			continue
		}
		pendingMap := fetchPendingMap(ctx, client, listURL, cfg.WorkerToken)
		for _, id := range ids {
			if _, seen := inFlight[id]; seen {
				continue
			}
			inFlight[id] = time.Now()
			pi, ok := pendingMap[id]
			if !ok {
				log.Printf("planner: pending input %s not found in list", id)
				continue
			}
			err := planOne(ctx, client, cfg, pi, statusURL)
			if err != nil {
				log.Printf("planner: failed planning id=%s: %v", id, err)
			}
		}
	}
}

func popPlanIDs(ctx context.Context, client *http.Client, popURL, token string, batch int) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s?max=%d", popURL, batch), nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Worker-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pop status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.IDs, nil
}

func fetchPendingMap(ctx context.Context, client *http.Client, listURL, token string) map[string]pendingInput {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return map[string]pendingInput{}
	}
	if token != "" {
		req.Header.Set("X-Worker-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]pendingInput{}
	}
	defer resp.Body.Close()
	var list []pendingInput
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return map[string]pendingInput{}
	}
	mp := make(map[string]pendingInput, len(list))
	for _, pi := range list {
		mp[fmt.Sprintf("%d", pi.ID)] = pi
	}
	return mp
}

func planOne(ctx context.Context, client *http.Client, cfg Config, pi pendingInput, statusURL string) error {
	reqPath := filepath.Join(cfg.InputDir, pi.Filename)
	snap, err := plan.Generate(
		cfg.InputDir,
		cfg.CacheDir,
		cfg.PythonVersion,
		cfg.PlatformTag,
		cfg.IndexURL,
		cfg.ExtraIndexURL,
		cfg.UpgradeStrategy,
		reqPath,
		cfg.ConstraintsPath,
		cfg.PackCatalog,
		cfg.CASStore(),
		cfg.CASRegistryURL,
		cfg.CASRegistryRepo,
	)
	statusBody := map[string]string{"status": "planned"}
	if err != nil {
		statusBody["status"] = "failed"
		statusBody["error"] = err.Error()
	}
	if postErr := updatePendingStatus(ctx, client, statusURL, cfg.WorkerToken, pi.ID, statusBody); postErr != nil {
		log.Printf("planner: status update failed for id %d: %v", pi.ID, postErr)
	}
	if err != nil {
		return err
	}
	if err := postPlan(ctx, client, cfg, snap); err != nil {
		return fmt.Errorf("post plan: %w", err)
	}
	if cfg.AutoBuild {
		if err := enqueueBuild(ctx, client, cfg, snap); err != nil {
			log.Printf("planner: auto-build enqueue failed for run %s: %v", snap.RunID, err)
		}
	}
	return nil
}

func updatePendingStatus(ctx context.Context, client *http.Client, statusURL, token string, id int64, body map[string]string) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%d", statusURL, id), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Worker-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func postPlan(ctx context.Context, client *http.Client, cfg Config, snap plan.Snapshot) error {
	if cfg.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL not set")
	}
	url := strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/plan"
	body := map[string]any{"run_id": snap.RunID, "plan": snap.Plan, "dag": snap.DAG}
	data, _ := json.Marshal(body)
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
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plan post status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func enqueueBuild(ctx context.Context, client *http.Client, cfg Config, snap plan.Snapshot) error {
	if cfg.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL not set")
	}
	url := strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/queue/enqueue"
	for _, node := range snap.Plan {
		if strings.ToLower(node.Action) != "build" {
			continue
		}
		body := map[string]any{
			"package":      node.Name,
			"version":      node.Version,
			"python_tag":   node.PythonTag,
			"platform_tag": node.PlatformTag,
		}
		data, _ := json.Marshal(body)
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
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("build enqueue status %d: %s", resp.StatusCode, string(b))
		}
		resp.Body.Close()
	}
	return nil
}
