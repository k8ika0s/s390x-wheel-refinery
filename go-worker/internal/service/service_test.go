package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

func TestPlanEndpointGeneratesPlan(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		HTTPAddr:      ":0",
		InputDir:      filepath.Join(dir, "input"),
		OutputDir:     filepath.Join(dir, "output"),
		CacheDir:      filepath.Join(dir, "cache"),
		PythonVersion: "3.11",
		PlatformTag:   "manylinux2014_s390x",
	}
	if err := os.MkdirAll(cfg.InputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// add a pure wheel
	if err := os.WriteFile(filepath.Join(cfg.InputDir, "purepkg-1.0.0-py3-none-any.whl"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := BuildWorker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/plan", func(wr http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			snap, err := plan.Generate(cfg.InputDir, cfg.CacheDir, cfg.PythonVersion, cfg.PlatformTag, cfg.IndexURL, cfg.ExtraIndexURL)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeJSON(wr, http.StatusOK, snap)
		case http.MethodGet:
			snap, err := plan.Load(filepath.Join(cfg.CacheDir, "plan.json"))
			if err != nil {
				wr.WriteHeader(http.StatusNotFound)
				return
			}
			writeJSON(wr, http.StatusOK, snap)
		default:
			wr.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	_ = w // keep worker referenced (unused here)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/plan", "application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	var snapResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&snapResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	getResp, err := http.Get(ts.URL + "/plan")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", getResp.StatusCode)
	}
}
