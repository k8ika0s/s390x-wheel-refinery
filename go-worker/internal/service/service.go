package service

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

// Run starts the worker HTTP server (stub for now).
func Run() error {
	cfg := overlaySettingsFromControlPlane(fromEnv())
	w, err := BuildWorker(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.PlanPollEnabled && cfg.ControlPlaneURL != "" {
		popURL := cfg.PlanPopURL
		if popURL == "" {
			popURL = strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/pending-inputs/pop"
		}
		statusURL := cfg.PlanStatusURL
		if statusURL == "" {
			statusURL = strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/pending-inputs/status"
		}
		listURL := cfg.PlanListURL
		if listURL == "" {
			listURL = strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/pending-inputs"
		}
		go plannerLoop(ctx, cfg, popURL, statusURL, listURL)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(wr http.ResponseWriter, r *http.Request) {
		wr.WriteHeader(http.StatusOK)
		_, _ = wr.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/ready", func(wr http.ResponseWriter, r *http.Request) {
		wr.WriteHeader(http.StatusOK)
		_, _ = wr.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("/trigger", func(wr http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			wr.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if cfg.WorkerToken != "" {
			tok := r.Header.Get("X-Worker-Token")
			if tok == "" {
				tok = r.URL.Query().Get("token")
			}
			if tok != cfg.WorkerToken {
				wr.WriteHeader(http.StatusForbidden)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		if err := w.RunOnce(ctx); err != nil {
			wr.WriteHeader(http.StatusInternalServerError)
			_, _ = wr.Write([]byte(err.Error()))
			return
		}
		wr.WriteHeader(http.StatusOK)
		_, _ = wr.Write([]byte(`{"detail":"worker ran"}`))
	})
	mux.HandleFunc("/plan", func(wr http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			p := filepath.Join(cfg.OutputDir, "plan.json")
			snap, err := plan.Load(p)
			if err != nil {
				p = filepath.Join(cfg.CacheDir, "plan.json")
				snap, err = plan.Load(p)
			}
			if err != nil {
				wr.WriteHeader(http.StatusNotFound)
				_, _ = wr.Write([]byte(`{"error":"plan not found"}`))
				return
			}
			writeJSON(wr, http.StatusOK, snap)
		case http.MethodPost:
			if cfg.InputDir == "" {
				wr.WriteHeader(http.StatusBadRequest)
				_, _ = wr.Write([]byte(`{"error":"input dir disabled; use pending-input planning"}`))
				return
			}
			if cfg.WorkerToken != "" {
				tok := r.Header.Get("X-Worker-Token")
				if tok == "" {
					tok = r.URL.Query().Get("token")
				}
				if tok != cfg.WorkerToken {
					wr.WriteHeader(http.StatusForbidden)
					return
				}
			}
			snap, err := plan.Generate(
				cfg.InputDir,
				cfg.CacheDir,
				cfg.PythonVersion,
				cfg.PlatformTag,
				cfg.IndexURL,
				cfg.ExtraIndexURL,
				cfg.UpgradeStrategy,
				cfg.RequirementsPath,
				cfg.ConstraintsPath,
				cfg.PackCatalog,
				cfg.CASStore(),
				cfg.CASRegistryURL,
				cfg.CASRegistryRepo,
			)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				_, _ = wr.Write([]byte(err.Error()))
				return
			}
			writeJSON(wr, http.StatusOK, snap)
		default:
			wr.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
	go func() {
		<-context.Background().Done()
		_ = srv.Shutdown(context.Background())
	}()
	log.Printf("starting worker on %s", srv.Addr)
	return srv.ListenAndServe()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// overlaySettingsFromControlPlane fetches settings from control-plane and applies
// pool sizes when available, so worker concurrency aligns with UI-configured values.
func overlaySettingsFromControlPlane(cfg Config) Config {
	if cfg.ControlPlaneURL == "" {
		return cfg
	}
	url := strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/settings"
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return cfg
	}
	if cfg.ControlPlaneToken != "" {
		req.Header.Set("X-Worker-Token", cfg.ControlPlaneToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("settings fetch failed: %v", err)
		return cfg
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("settings fetch status %d", resp.StatusCode)
		return cfg
	}
	var payload struct {
		PlanPoolSize  int `json:"plan_pool_size"`
		BuildPoolSize int `json:"build_pool_size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Printf("settings decode failed: %v", err)
		return cfg
	}
	if payload.PlanPoolSize > 0 {
		cfg.PlanPoolSize = payload.PlanPoolSize
	}
	if payload.BuildPoolSize > 0 {
		cfg.BuildPoolSize = payload.BuildPoolSize
	}
	return cfg
}
