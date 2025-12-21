package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func defaultWorkerRunID(workerID string) string {
	return fmt.Sprintf("%s-%d", workerID, time.Now().UnixNano())
}

func heartbeatLoop(ctx context.Context, cfg Config, w *Worker, workerID, runID string, planPool, buildPool *atomic.Int32) {
	if cfg.ControlPlaneURL == "" {
		return
	}
	intervalSec := cfg.HeartbeatIntervalSec
	if intervalSec <= 0 {
		intervalSec = 15
	}
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()
	send := func() {
		buildPoolSize := cfg.BuildPoolSize
		if buildPool != nil && buildPool.Load() > 0 {
			buildPoolSize = int(buildPool.Load())
		}
		planPoolSize := cfg.PlanPoolSize
		if planPool != nil && planPool.Load() > 0 {
			planPoolSize = int(planPool.Load())
		}
		payload := map[string]any{
			"worker_id":              workerID,
			"run_id":                 runID,
			"active_builds":          int(w.activeBuilds.Load()),
			"build_pool_size":        buildPoolSize,
			"plan_pool_size":         planPoolSize,
			"heartbeat_interval_sec": intervalSec,
		}
		if err := postHeartbeat(ctx, cfg, payload); err != nil {
			log.Printf("heartbeat: %v", err)
		}
	}
	send()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func postHeartbeat(ctx context.Context, cfg Config, payload map[string]any) error {
	url := strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/worker/heartbeat"
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.ControlPlaneToken != "" {
		req.Header.Set("X-Worker-Token", cfg.ControlPlaneToken)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
