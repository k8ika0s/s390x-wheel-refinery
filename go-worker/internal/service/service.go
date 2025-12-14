package service

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

// Run starts the worker HTTP server (stub for now).
func Run() error {
	cfg := fromEnv()
	w, err := BuildWorker(cfg)
	if err != nil {
		return err
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
