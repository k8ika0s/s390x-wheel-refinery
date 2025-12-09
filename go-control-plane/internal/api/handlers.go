package api

import (
	"encoding/json"
	"net/http"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
)

// Handler wires HTTP routes to store/queue backends.
type Handler struct {
	Store store.Store
	Queue queue.Backend
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.health)
	mux.HandleFunc("/api/config", h.notImplemented)
	mux.HandleFunc("/api/summary", h.notImplemented)
	mux.HandleFunc("/api/recent", h.notImplemented)
	mux.HandleFunc("/api/history", h.notImplemented)
	mux.HandleFunc("/api/plan", h.notImplemented)
	mux.HandleFunc("/api/manifest", h.notImplemented)
	mux.HandleFunc("/api/artifacts", h.notImplemented)
	mux.HandleFunc("/api/queue", h.notImplemented)
	mux.HandleFunc("/api/queue/stats", h.notImplemented)
	mux.HandleFunc("/api/queue/enqueue", h.notImplemented)
	mux.HandleFunc("/api/queue/clear", h.notImplemented)
	mux.HandleFunc("/api/worker/trigger", h.notImplemented)
	mux.HandleFunc("/api/hints", h.notImplemented)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
