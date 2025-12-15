package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
)

// Handler wires HTTP routes to store/queue backends.
type Handler struct {
	Store  store.Store
	Queue  queue.Backend
	Config config.Config
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.health)
	mux.HandleFunc("/api/ready", h.ready)
	mux.HandleFunc("/api/metrics", h.metrics)
	mux.HandleFunc("/api/config", h.config)
	mux.HandleFunc("/api/session/token", h.sessionToken)
	mux.HandleFunc("/api/summary", h.summary)
	mux.HandleFunc("/api/recent", h.recent)
	mux.HandleFunc("/api/history", h.history)
	mux.HandleFunc("/api/package/", h.packageSummary)
	mux.HandleFunc("/api/event/", h.eventByVersion)
	mux.HandleFunc("/api/failures", h.failures)
	mux.HandleFunc("/api/variants/", h.variants)
	mux.HandleFunc("/api/top-failures", h.topFailures)
	mux.HandleFunc("/api/top-slowest", h.topSlowest)
	mux.HandleFunc("/api/plan", h.plan)
	mux.HandleFunc("/api/plan/compute", h.planCompute)
	mux.HandleFunc("/api/manifest", h.manifest)
	mux.HandleFunc("/api/artifacts", h.artifacts)
	mux.HandleFunc("/api/queue", h.queueList)
	mux.HandleFunc("/api/queue/stats", h.queueStats)
	mux.HandleFunc("/api/queue/enqueue", h.queueEnqueue)
	mux.HandleFunc("/api/queue/clear", h.queueClear)
	mux.HandleFunc("/api/hints", h.hints)
	mux.HandleFunc("/api/hints/", h.hintByID)
	mux.HandleFunc("/api/logs/", h.logsByNameVersion)
	mux.HandleFunc("/api/logs/search", h.logsSearch)
	mux.HandleFunc("/api/logs", h.logsIngest)
	mux.HandleFunc("/api/logs/stream", h.logsStream)
	mux.HandleFunc("/api/worker/trigger", h.workerTrigger)
	mux.HandleFunc("/api/worker/smoke", h.workerSmoke)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	status := map[string]any{"status": "ok"}
	if err := h.ping(ctx); err != nil {
		status["status"] = "degraded"
		status["detail"] = err.Error()
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	type queueMetrics struct {
		Backend       string `json:"backend"`
		Length        int    `json:"length"`
		OldestAgeSec  int64  `json:"oldest_age_seconds,omitempty"`
		ConsumerState string `json:"consumer_state,omitempty"`
	}
	type dbMetrics struct {
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Queue metrics
	qm := queueMetrics{Backend: h.Config.QueueBackend}
	if h.Queue != nil {
		stats, err := h.Queue.Stats(ctx)
		if err == nil {
			qm.Length = stats.Length
			qm.OldestAgeSec = stats.OldestAge
		} else {
			qm.ConsumerState = fmt.Sprintf("stats_error: %v", err)
		}
	}

	// DB metrics
	dbm := dbMetrics{Status: "unknown"}
	if pinger, ok := h.Store.(interface{ Ping(context.Context) error }); ok {
		if err := pinger.Ping(ctx); err != nil {
			dbm.Status = "degraded"
			dbm.Error = err.Error()
		} else {
			dbm.Status = "ok"
		}
	} else {
		dbm.Status = "n/a"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"summary": map[string]string{
			"title":       "Control-plane metrics",
			"description": "Quick glance at queue depth and database health. Export Prometheus metrics separately if enabled.",
		},
		"queue": qm,
		"db":    dbm,
	})
}

func (h *Handler) sessionToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "worker_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"detail": "token set"})
}

func (h *Handler) config(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"http_addr":        h.Config.HTTPAddr,
		"queue_backend":    h.Config.QueueBackend,
		"queue_file":       h.Config.QueueFile,
		"redis_url":        h.Config.RedisURL,
		"redis_key":        h.Config.RedisKey,
		"kafka_brokers":    h.Config.KafkaBrokers,
		"kafka_topic":      h.Config.KafkaTopic,
		"db":               "postgres",
		"worker_webhook":   h.Config.WorkerWebhookURL != "",
		"worker_local_cmd": h.Config.WorkerLocalCmd != "",
	})
}

func (h *Handler) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("failure_limit"), 20, 200)
	sum, err := h.Store.Summary(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (h *Handler) recent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	limit := parseIntDefault(q.Get("limit"), 50, 500)
	offset := parseIntDefault(q.Get("offset"), 0, 10_000)
	pkg := q.Get("package")
	status := q.Get("status")
	events, err := h.Store.Recent(r.Context(), limit, offset, pkg, status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		filter := store.HistoryFilter{
			Package: q.Get("package"),
			Status:  q.Get("status"),
			RunID:   q.Get("run_id"),
			FromTs:  int64(parseIntDefault(q.Get("from"), 0, 0)),
			ToTs:    int64(parseIntDefault(q.Get("to"), 0, 0)),
			Limit:   parseIntDefault(q.Get("limit"), 50, 500),
			Offset:  parseIntDefault(q.Get("offset"), 0, 10_000),
		}
		res, err := h.Store.History(r.Context(), filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	case http.MethodPost:
		var evt store.Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if evt.Name == "" || evt.Version == "" || evt.Status == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, version, and status are required"})
			return
		}
		if evt.Timestamp == 0 {
			evt.Timestamp = time.Now().Unix()
		}
		if err := h.Store.RecordEvent(r.Context(), evt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "event recorded"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) packageSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	name := parts[2]
	ps, err := h.Store.PackageSummary(r.Context(), name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (h *Handler) eventByVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name/version required"})
		return
	}
	name, version := parts[2], parts[3]
	evt, err := h.Store.LatestEvent(r.Context(), name, version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, evt)
}

func (h *Handler) failures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	name := r.URL.Query().Get("name")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50, 500)
	res, err := h.Store.Failures(r.Context(), name, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) variants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	name := parts[2]
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100, 500)
	res, err := h.Store.Variants(r.Context(), name, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) topFailures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 10, 200)
	res, err := h.Store.TopFailures(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) topSlowest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 10, 200)
	res, err := h.Store.TopSlowest(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) plan(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := h.Store.Plan(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	case http.MethodPost:
		var body struct {
			RunID string           `json:"run_id"`
			Plan  []store.PlanNode `json:"plan"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if len(body.Plan) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan required"})
			return
		}
		if err := h.Store.SavePlan(r.Context(), body.RunID, body.Plan); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "plan saved"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) manifest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := parseIntDefault(r.URL.Query().Get("limit"), 200, 1000)
		res, err := h.Store.Manifest(r.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	case http.MethodPost:
		var entries []store.ManifestEntry
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if len(entries) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "manifest entries required"})
			return
		}
		if err := h.Store.SaveManifest(r.Context(), entries); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "manifest saved"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// planCompute proxies a plan computation to the worker (if configured).
func (h *Handler) planCompute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	snap, err := h.callWorkerPlan(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	// Persist plan snapshot if provided
	var nodes []store.PlanNode
	if planArr, ok := snap["plan"].([]any); ok {
		for _, raw := range planArr {
			if m, ok := raw.(map[string]any); ok {
				nodes = append(nodes, store.PlanNode{
					Name:          toString(m["name"]),
					Version:       toString(m["version"]),
					PythonVersion: toString(m["python_version"]),
					PythonTag:     toString(m["python_tag"]),
					PlatformTag:   toString(m["platform_tag"]),
					Action:        toString(m["action"]),
				})
			}
		}
	}
	if len(nodes) > 0 {
		_ = h.Store.SavePlan(ctx, toString(snap["run_id"]), nodes)
	}
	writeJSON(w, http.StatusOK, snap)
}

func (h *Handler) artifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200, 1000)
	res, err := h.Store.Artifacts(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) logsIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if r.ContentLength > 1_000_000 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "log too large"})
		return
	}
	var le store.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&le); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if le.Name == "" || le.Version == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and version required"})
		return
	}
	le.Timestamp = time.Now().Unix()
	if err := h.Store.PutLog(r.Context(), le); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"detail": "log saved"})
}

func (h *Handler) queueList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	items, err := h.Queue.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) queueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	stats, err := h.Queue.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) queueEnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req queue.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Package == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "package required"})
		return
	}
	if err := h.Queue.Enqueue(r.Context(), req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"detail": "enqueued"})
}

func (h *Handler) queueClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.Queue.Clear(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"detail": "cleared"})
}

func (h *Handler) workerTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx := r.Context()
	detail := []string{}
	if h.Config.WorkerWebhookURL != "" {
		if err := h.callWorkerWebhook(ctx, &detail); err != nil {
			detail = append(detail, "webhook error: "+err.Error())
		}
	}
	if h.Config.WorkerLocalCmd != "" {
		if err := h.callWorkerLocal(ctx, &detail); err != nil {
			detail = append(detail, "local worker error: "+err.Error())
		}
	}
	stats, _ := h.Queue.Stats(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"detail": strings.Join(detail, "; "), "queue_length": stats.Length})
}

func (h *Handler) workerSmoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	stats, _ := h.Queue.Stats(ctx)
	writeJSON(w, http.StatusOK, map[string]any{"detail": "smoke-ok", "queue_length": stats.Length})
}

func (h *Handler) hints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hints, err := h.Store.ListHints(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, hints)
	case http.MethodPost:
		var hint store.Hint
		if err := json.NewDecoder(r.Body).Decode(&hint); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if hint.ID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.Store.PutHint(r.Context(), hint); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "created"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) hintByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/hints/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		hint, err := h.Store.GetHint(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, hint)
	case http.MethodPut:
		var hint store.Hint
		if err := json.NewDecoder(r.Body).Decode(&hint); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		hint.ID = id
		if err := h.Store.PutHint(r.Context(), hint); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "updated"})
	case http.MethodDelete:
		if err := h.Store.DeleteHint(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) logsByNameVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// Path: /api/logs/{name}/{version}
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name/version required"})
		return
	}
	name, version := parts[2], parts[3]
	logEntry, err := h.Store.GetLog(r.Context(), name, version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, logEntry)
}

func (h *Handler) logsSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	q := r.URL.Query().Get("q")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50, 200)
	results, err := h.Store.SearchLogs(r.Context(), q, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) logsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name/version required"})
		return
	}
	name, version := parts[2], parts[3]
	entry, err := h.Store.GetLog(r.Context(), name, version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("event: log\n"))
	payload, _ := json.Marshal(entry)
	_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func parseIntDefault(val string, def int, max int) int {
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil || i <= 0 {
		return def
	}
	if max > 0 && i > max {
		return max
	}
	return i
}

func splitPath(p string) []string {
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return parts
}

func (h *Handler) requireWorkerToken(r *http.Request) error {
	if h.Config.WorkerToken == "" {
		return nil
	}
	tok := r.Header.Get("X-Worker-Token")
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	if tok != h.Config.WorkerToken {
		return fmt.Errorf("invalid worker token")
	}
	return nil
}

func (h *Handler) ping(ctx context.Context) error {
	if pinger, ok := h.Store.(interface{ Ping(context.Context) error }); ok {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
	}
	if _, err := h.Queue.Stats(ctx); err != nil {
		return err
	}
	return nil
}

func (h *Handler) callWorkerWebhook(ctx context.Context, detail *[]string) error {
	payload := map[string]string{"action": "drain"}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.Config.WorkerWebhookURL, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if h.Config.WorkerToken != "" {
		req.Header.Set("X-Worker-Token", h.Config.WorkerToken)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	*detail = append(*detail, "webhook status "+resp.Status)
	return nil
}

func (h *Handler) callWorkerLocal(ctx context.Context, detail *[]string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.Config.WorkerLocalCmd, "drain")
	if err := cmd.Run(); err != nil {
		return err
	}
	*detail = append(*detail, "local worker triggered")
	return nil
}

func (h *Handler) callWorkerPlan(ctx context.Context) (map[string]any, error) {
	url := h.Config.WorkerPlanURL
	if url == "" && h.Config.WorkerWebhookURL != "" {
		url = strings.Replace(h.Config.WorkerWebhookURL, "/trigger", "/plan", 1)
	}
	if url == "" {
		return nil, fmt.Errorf("worker plan URL not configured")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if h.Config.WorkerToken != "" {
		req.Header.Set("X-Worker-Token", h.Config.WorkerToken)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var snap map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, err
	}
	return snap, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
