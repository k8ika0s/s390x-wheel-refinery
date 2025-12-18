package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/objectstore"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/settings"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
	"gopkg.in/yaml.v3"
)

// Handler wires HTTP routes to store/queue backends.
type Handler struct {
	Store      store.Store
	Queue      queue.Backend
	PlanQ      queue.PlanQueueBackend
	InputStore objectstore.Store
	Config     config.Config
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.health)
	mux.HandleFunc("/api/ready", h.ready)
	mux.HandleFunc("/api/metrics", h.metrics)
	mux.HandleFunc("/metrics", h.promMetrics)
	mux.HandleFunc("/api/config", h.config)
	mux.HandleFunc("/api/settings", h.settings)
	mux.HandleFunc("/api/pending-inputs", h.pendingInputs)
	mux.HandleFunc("/api/pending-inputs/clear", h.pendingInputsClear)
	mux.HandleFunc("/api/pending-inputs/", h.pendingInputAction)
	mux.HandleFunc("/api/pending-inputs/pop", h.pendingInputPop)
	mux.HandleFunc("/api/pending-inputs/status/", h.pendingInputStatus)
	mux.HandleFunc("/api/plan-queue/clear", h.planQueueClear)
	mux.HandleFunc("/api/requirements/upload", h.requirementsUpload)
	mux.HandleFunc("/api/wheels/upload", h.wheelsUpload)
	mux.HandleFunc("/api/builds", h.builds)
	mux.HandleFunc("/api/builds/status", h.buildStatusUpdate)
	mux.HandleFunc("/api/build-queue/pop", h.buildQueuePop)
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
	mux.HandleFunc("/api/plan/latest", h.planLatest)
	mux.HandleFunc("/api/plan/", h.planByID)
	mux.HandleFunc("/api/plans", h.plans)
	mux.HandleFunc("/api/plan/compute", h.planCompute)
	mux.HandleFunc("/api/manifest", h.manifest)
	mux.HandleFunc("/api/artifacts", h.artifacts)
	mux.HandleFunc("/api/queue", h.queueList)
	mux.HandleFunc("/api/queue/stats", h.queueStats)
	mux.HandleFunc("/api/queue/enqueue", h.queueEnqueue)
	mux.HandleFunc("/api/queue/clear", h.queueClear)
	mux.HandleFunc("/api/hints", h.hints)
	mux.HandleFunc("/api/hints/bulk", h.hintsBulk)
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
	type pendingMetrics struct {
		Count     int `json:"count"`
		PlanQueue int `json:"plan_queue"`
	}
	type buildMetrics struct {
		Length       int   `json:"length"`
		OldestAgeSec int64 `json:"oldest_age_seconds,omitempty"`
		Pending      int   `json:"pending"`
		Retry        int   `json:"retry"`
	}
	type hintMetrics struct {
		Count int `json:"count"`
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	sum := store.Summary{StatusCounts: map[string]int{}, Failures: []store.Event{}}
	if h.Store != nil {
		if s, err := h.Store.Summary(ctx, 10); err == nil {
			sum = s
		}
	}

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

	pm := pendingMetrics{}
	if h.Store != nil {
		if list, err := h.Store.ListPendingInputs(ctx, ""); err == nil {
			pm.Count = len(list)
		}
	}
	if pq, ok := h.PlanQ.(interface {
		Len(context.Context) (int64, error)
	}); ok && pq != nil {
		if n, err := pq.Len(ctx); err == nil {
			pm.PlanQueue = int(n)
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

	buildStats := buildMetrics{}
	if h.Store != nil {
		list, err := h.Store.ListBuilds(ctx, "", 500)
		if err == nil {
			var oldest int64
			for _, b := range list {
				if strings.EqualFold(b.Status, "pending") {
					buildStats.Pending++
				}
				if strings.EqualFold(b.Status, "retry") {
					buildStats.Retry++
				}
				buildStats.Length++
				if oldest == 0 || b.OldestAgeSec > oldest {
					oldest = b.OldestAgeSec
				}
			}
			buildStats.OldestAgeSec = oldest
		}
	}
	hm := hintMetrics{}
	if counter, ok := h.Store.(interface {
		HintCount(context.Context) (int, error)
	}); ok && counter != nil {
		if count, err := counter.HintCount(ctx); err == nil {
			hm.Count = count
		}
	}
	poolPlan := 0
	poolBuild := 0
	if h.Store != nil {
		if s, err := h.Store.GetSettings(ctx); err == nil {
			poolPlan = s.PlanPoolSize
			poolBuild = s.BuildPoolSize
		}
	} else {
		s := settings.Load(h.Config.SettingsPath)
		poolPlan = s.PlanPoolSize
		poolBuild = s.BuildPoolSize
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": map[string]any{
			"title":       "Control-plane metrics",
			"description": "Queue depth, DB health, and recent failure snapshot for quick triage.",
			"updated_at":  time.Now().Unix(),
		},
		"queue":           qm,
		"pending":         pm,
		"build":           buildStats,
		"hints":           hm,
		"db":              dbm,
		"status_counts":   sum.StatusCounts,
		"recent_failures": sum.Failures,
		"auto_plan":       h.Config.AutoPlan,
		"auto_build":      h.Config.AutoBuild,
		"plan_pool_size":  poolPlan,
		"build_pool_size": poolBuild,
	})
}

// promMetrics exposes a simple Prometheus text exposition for quick scrapes.
func (h *Handler) promMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	sum, _ := h.Store.Summary(ctx, 10)
	qstats, _ := h.Queue.Stats(ctx)
	buildStats, _ := h.Queue.Stats(ctx)
	poolPlan := 0
	poolBuild := 0
	if h.Store != nil {
		if s, err := h.Store.GetSettings(ctx); err == nil {
			poolPlan = s.PlanPoolSize
			poolBuild = s.BuildPoolSize
		}
	} else {
		s := settings.Load(h.Config.SettingsPath)
		poolPlan = s.PlanPoolSize
		poolBuild = s.BuildPoolSize
	}
	dbOK := 0
	if pinger, ok := h.Store.(interface{ Ping(context.Context) error }); ok {
		if err := pinger.Ping(ctx); err == nil {
			dbOK = 1
		}
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# HELP refinery_queue_length Number of items in the retry/build queue.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_queue_length gauge\n")
	fmt.Fprintf(&buf, "refinery_queue_length{backend=%q} %d\n", h.Config.QueueBackend, qstats.Length)
	fmt.Fprintf(&buf, "# HELP refinery_queue_oldest_seconds Age in seconds of the oldest queued item.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_queue_oldest_seconds gauge\n")
	fmt.Fprintf(&buf, "refinery_queue_oldest_seconds %d\n", qstats.OldestAge)
	fmt.Fprintf(&buf, "# HELP refinery_build_queue_length Build queue length.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_build_queue_length gauge\n")
	fmt.Fprintf(&buf, "refinery_build_queue_length %d\n", buildStats.Length)
	fmt.Fprintf(&buf, "# HELP refinery_build_queue_oldest_seconds Age in seconds of the oldest build item.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_build_queue_oldest_seconds gauge\n")
	fmt.Fprintf(&buf, "refinery_build_queue_oldest_seconds %d\n", buildStats.OldestAge)
	fmt.Fprintf(&buf, "# HELP refinery_pool_size_plan Configured plan pool size.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_pool_size_plan gauge\n")
	fmt.Fprintf(&buf, "refinery_pool_size_plan %d\n", poolPlan)
	fmt.Fprintf(&buf, "# HELP refinery_pool_size_build Configured build pool size.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_pool_size_build gauge\n")
	fmt.Fprintf(&buf, "refinery_pool_size_build %d\n", poolBuild)
	if pq, ok := h.PlanQ.(interface {
		Len(context.Context) (int64, error)
	}); ok && pq != nil {
		if n, err := pq.Len(ctx); err == nil {
			fmt.Fprintf(&buf, "# HELP refinery_plan_queue_length Items waiting for planning.\n")
			fmt.Fprintf(&buf, "# TYPE refinery_plan_queue_length gauge\n")
			fmt.Fprintf(&buf, "refinery_plan_queue_length %d\n", n)
		}
	}
	if list, err := h.Store.ListPendingInputs(ctx, ""); err == nil {
		fmt.Fprintf(&buf, "# HELP refinery_pending_inputs_total Pending uploads awaiting planning.\n")
		fmt.Fprintf(&buf, "# TYPE refinery_pending_inputs_total gauge\n")
		fmt.Fprintf(&buf, "refinery_pending_inputs_total %d\n", len(list))
	}
	fmt.Fprintf(&buf, "# HELP refinery_db_up Database connectivity (1=up,0=down).\n")
	fmt.Fprintf(&buf, "# TYPE refinery_db_up gauge\n")
	fmt.Fprintf(&buf, "refinery_db_up %d\n", dbOK)
	fmt.Fprintf(&buf, "# HELP refinery_status_count Recent status counts.\n")
	fmt.Fprintf(&buf, "# TYPE refinery_status_count gauge\n")
	for k, v := range sum.StatusCounts {
		fmt.Fprintf(&buf, "refinery_status_count{status=%q} %d\n", k, v)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
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
	currentSettings := settings.Load(h.Config.SettingsPath)
	autoPlan := settings.BoolValue(currentSettings.AutoPlan)
	autoBuild := settings.BoolValue(currentSettings.AutoBuild)
	writeJSON(w, http.StatusOK, map[string]any{
		"http_addr":        h.Config.HTTPAddr,
		"queue_backend":    h.Config.QueueBackend,
		"queue_file":       h.Config.QueueFile,
		"redis_url":        h.Config.RedisURL,
		"redis_key":        h.Config.RedisKey,
		"plan_redis_key":   h.Config.PlanRedisKey,
		"kafka_brokers":    h.Config.KafkaBrokers,
		"kafka_topic":      h.Config.KafkaTopic,
		"db":               "postgres",
		"worker_webhook":   h.Config.WorkerWebhookURL != "",
		"worker_local_cmd": h.Config.WorkerLocalCmd != "",
		"input_object": map[string]any{
			"endpoint": h.Config.ObjectStoreEndpoint,
			"bucket":   h.Config.ObjectStoreBucket,
			"prefix":   h.Config.InputObjectPrefix,
		},
		"settings_path": h.Config.SettingsPath,
		"hints_dir":     h.Config.HintsDir,
		"hints_seed":    h.Config.SeedHints,
		"settings":      currentSettings,
		"auto_plan":     autoPlan,
		"auto_build":    autoBuild,
	})
}

func lintRequirements(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty file")
	}
	if len(data) > 128*1024 {
		return fmt.Errorf("file too large (>128KB)")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return fmt.Errorf("file contains null bytes")
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) > 2000 {
		return fmt.Errorf("too many lines (>2000)")
	}
	for i, l := range lines {
		if len(l) > 800 {
			return fmt.Errorf("line %d too long (>800 chars)", i+1)
		}
		for _, b := range l {
			// allow printable ASCII, tabs, and '#'/punctuation; reject control chars.
			if b < 9 || b == 11 || b == 12 || b > 126 {
				return fmt.Errorf("invalid character on line %d", i+1)
			}
		}
	}
	return nil
}

type requirementSpec struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type wheelMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PythonTag   string `json:"python_tag"`
	AbiTag      string `json:"abi_tag"`
	PlatformTag string `json:"platform_tag"`
}

func normalizeName(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
}

func looksLikeHTMLOrScript(data []byte) bool {
	sample := strings.ToLower(string(data))
	if len(sample) > 4096 {
		sample = sample[:4096]
	}
	for _, marker := range []string{"<script", "<html", "<?php", "<iframe", "javascript:"} {
		if strings.Contains(sample, marker) {
			return true
		}
	}
	return false
}

func parseRequirements(data []byte) []requirementSpec {
	lines := strings.Split(string(data), "\n")
	out := make([]requirementSpec, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-c ") || strings.HasPrefix(line, "--constraint") {
			continue
		}
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		name := line
		version := ""
		for _, op := range []string{"==", ">=", "~="} {
			if idx := strings.Index(line, op); idx > 0 {
				name = strings.TrimSpace(line[:idx])
				version = line[idx:]
				if op == "==" {
					version = strings.TrimPrefix(version, "==")
				}
				break
			}
		}
		name = normalizeName(name)
		if name == "" {
			continue
		}
		out = append(out, requirementSpec{Name: name, Version: version})
	}
	return out
}

func parseWheelFilename(name string) (wheelMeta, error) {
	base := strings.TrimSuffix(name, ".whl")
	parts := strings.Split(base, "-")
	if len(parts) < 5 {
		return wheelMeta{}, fmt.Errorf("invalid wheel filename: %s", name)
	}
	platform := parts[len(parts)-1]
	abi := parts[len(parts)-2]
	py := parts[len(parts)-3]
	version := parts[len(parts)-4]
	pkgParts := parts[:len(parts)-4]
	pkg := strings.ReplaceAll(strings.Join(pkgParts, "-"), "_", "-")
	return wheelMeta{
		Name:        pkg,
		Version:     version,
		PythonTag:   py,
		AbiTag:      abi,
		PlatformTag: platform,
	}, nil
}

func parseRequiresDist(meta string) []requirementSpec {
	lines := strings.Split(meta, "\n")
	out := make([]requirementSpec, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "requires-dist:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			val := strings.TrimSpace(parts[1])
			if semi := strings.Index(val, ";"); semi != -1 {
				val = strings.TrimSpace(val[:semi])
			}
			raw := val
			name := val
			version := ""
			if idx := strings.Index(raw, "("); idx != -1 {
				name = strings.TrimSpace(raw[:idx])
				spec := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw[idx:]), "("), ")")
				spec = strings.TrimSpace(spec)
				if strings.HasPrefix(spec, "==") || strings.HasPrefix(spec, ">=") || strings.HasPrefix(spec, "~=") {
					version = spec
					if strings.HasPrefix(version, "==") {
						version = strings.TrimPrefix(version, "==")
					}
				}
			} else {
				for _, op := range []string{"==", ">=", "~="} {
					if idx := strings.Index(raw, op); idx > 0 {
						name = strings.TrimSpace(raw[:idx])
						version = strings.TrimPrefix(strings.TrimSpace(raw[idx:]), "==")
						break
					}
				}
			}
			name = normalizeName(name)
			if name == "" {
				continue
			}
			out = append(out, requirementSpec{Name: name, Version: version})
		}
	}
	return out
}

func readWheelMetadata(data []byte) ([]requirementSpec, error) {
	reader := bytes.NewReader(data)
	zr, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, err
	}
	var meta []byte
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "METADATA") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			meta = buf.Bytes()
			break
		}
	}
	if len(meta) == 0 {
		return nil, nil
	}
	return parseRequiresDist(string(meta)), nil
}

func validateWheelArchive(data []byte) error {
	reader := bytes.NewReader(data)
	zr, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid wheel archive: %w", err)
	}
	if len(zr.File) == 0 {
		return fmt.Errorf("wheel archive is empty")
	}
	if len(zr.File) > 5000 {
		return fmt.Errorf("wheel archive has too many files")
	}
	for _, f := range zr.File {
		name := f.Name
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			return fmt.Errorf("wheel contains unsafe path: %s", name)
		}
	}
	return nil
}

func inputObjectKey(prefix, digestHex, filename string) string {
	clean := strings.TrimSpace(filename)
	if clean == "" {
		clean = "input"
	}
	clean = strings.ReplaceAll(clean, "..", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	base := strings.Trim(prefix, "/")
	if base == "" {
		return fmt.Sprintf("%s/%s", digestHex, clean)
	}
	return fmt.Sprintf("%s/%s/%s", base, digestHex, clean)
}

func (h *Handler) requirementsUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.Config.ObjectStoreEndpoint == "" || h.Config.ObjectStoreBucket == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input store not configured"})
		return
	}
	if h.InputStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input store unavailable"})
		return
	}
	if _, ok := h.InputStore.(objectstore.NullStore); ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input store unavailable"})
		return
	}
	if err := r.ParseMultipartForm(256 << 10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 256<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read file"})
		return
	}
	if err := lintRequirements(data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if looksLikeHTMLOrScript(data) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file appears to contain HTML/script content"})
		return
	}
	sum := sha256.Sum256(data)
	digestHex := hex.EncodeToString(sum[:])
	key := inputObjectKey(h.Config.InputObjectPrefix, digestHex, header.Filename)
	if err := h.InputStore.Put(r.Context(), key, data, "text/plain"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	meta := map[string]any{
		"type":         "requirements",
		"requirements": parseRequirements(data),
	}
	metaJSON, _ := json.Marshal(meta)
	pi := store.PendingInput{
		Filename:     header.Filename,
		Digest:       "sha256:" + digestHex,
		SizeBytes:    int64(len(data)),
		Status:       "pending",
		SourceType:   "requirements",
		ObjectBucket: h.Config.ObjectStoreBucket,
		ObjectKey:    key,
		ContentType:  "text/plain",
		Metadata:     metaJSON,
	}
	var pendingID int64
	if h.Store != nil {
		if id, err := h.Store.AddPendingInput(r.Context(), pi); err == nil {
			pendingID = id
			if h.Config.AutoPlan && h.PlanQ != nil {
				_ = h.PlanQ.Enqueue(r.Context(), fmt.Sprintf("%d", pendingID))
				_ = h.Store.UpdatePendingInputStatus(r.Context(), pendingID, "planning", "")
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"detail":     "requirements uploaded",
		"bytes":      len(data),
		"filename":   header.Filename,
		"object_key": key,
		"pending_id": pendingID,
	})
}

func (h *Handler) wheelsUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.Config.ObjectStoreEndpoint == "" || h.Config.ObjectStoreBucket == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input store not configured"})
		return
	}
	if h.InputStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input store unavailable"})
		return
	}
	if _, ok := h.InputStore.(objectstore.NullStore); ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input store unavailable"})
		return
	}
	if err := r.ParseMultipartForm(256 << 10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".whl") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wheel file (.whl) required"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 256<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read file"})
		return
	}
	if err := validateWheelArchive(data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sum := sha256.Sum256(data)
	digestHex := hex.EncodeToString(sum[:])
	key := inputObjectKey(h.Config.InputObjectPrefix, digestHex, header.Filename)
	if err := h.InputStore.Put(r.Context(), key, data, "application/octet-stream"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	wmeta, err := parseWheelFilename(header.Filename)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	reqs, err := readWheelMetadata(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	meta := map[string]any{
		"type":     "wheel",
		"wheel":    wmeta,
		"requires": reqs,
	}
	metaJSON, _ := json.Marshal(meta)
	pi := store.PendingInput{
		Filename:     header.Filename,
		Digest:       "sha256:" + digestHex,
		SizeBytes:    int64(len(data)),
		Status:       "pending",
		SourceType:   "wheel",
		ObjectBucket: h.Config.ObjectStoreBucket,
		ObjectKey:    key,
		ContentType:  "application/octet-stream",
		Metadata:     metaJSON,
	}
	var pendingID int64
	if h.Store != nil {
		if id, err := h.Store.AddPendingInput(r.Context(), pi); err == nil {
			pendingID = id
			if h.Config.AutoPlan && h.PlanQ != nil {
				_ = h.PlanQ.Enqueue(r.Context(), fmt.Sprintf("%d", pendingID))
				_ = h.Store.UpdatePendingInputStatus(r.Context(), pendingID, "planning", "")
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"detail":     "wheel uploaded",
		"bytes":      len(data),
		"filename":   header.Filename,
		"object_key": key,
		"pending_id": pendingID,
	})
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Prefer DB-backed settings; fall back to file if no store.
		if h.Store != nil {
			s, err := h.Store.GetSettings(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, s)
			return
		}
		writeJSON(w, http.StatusOK, settings.Load(h.Config.SettingsPath))
	case http.MethodPost:
		var s settings.Settings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		// basic normalization
		if s.RecentLimit <= 0 {
			s.RecentLimit = 25
		}
		if s.PollMs < 0 {
			s.PollMs = 0
		}
		s = settings.ApplyDefaults(s)
		if h.Store != nil {
			if err := h.Store.SaveSettings(r.Context(), s); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		} else {
			// fallback to file persistence if no store is configured
			if err := settings.Save(h.Config.SettingsPath, s); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		// reflect auto flags into config defaults for this process lifetime
		h.Config.AutoPlan = settings.BoolValue(s.AutoPlan)
		h.Config.AutoBuild = settings.BoolValue(s.AutoBuild)
		writeJSON(w, http.StatusOK, s)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (h *Handler) pendingInputs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	list, err := h.Store.ListPendingInputs(r.Context(), "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) pendingInputsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if h.Store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not configured"})
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	list, err := h.Store.ListPendingInputs(r.Context(), status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cleared := 0
	for _, pi := range list {
		if _, err := h.Store.DeletePendingInput(r.Context(), pi.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		cleared++
	}
	writeJSON(w, http.StatusOK, map[string]any{"detail": "cleared pending inputs", "count": cleared})
}

func (h *Handler) pendingInputAction(w http.ResponseWriter, r *http.Request) {
	// URL: /api/pending-inputs/{id}/enqueue-plan
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/pending-inputs/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}
	idStr := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if action == "" && r.Method == http.MethodDelete {
		if err := h.requireWorkerToken(r); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if h.Store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not configured"})
			return
		}
		if _, err := h.Store.DeletePendingInput(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "pending input not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"detail": "deleted pending input", "id": id})
		return
	}
	switch action {
	case "enqueue-plan":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if h.PlanQ == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "plan queue not configured"})
			return
		}
		if err := h.PlanQ.Enqueue(r.Context(), fmt.Sprintf("%d", id)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_ = h.Store.UpdatePendingInputStatus(r.Context(), id, "planning", "")
		writeJSON(w, http.StatusOK, map[string]string{"detail": "enqueued for planning"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
	}
}

func (h *Handler) pendingInputPop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if h.PlanQ == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "plan queue not configured"})
		return
	}
	max := parseIntDefault(r.URL.Query().Get("max"), 1, 100)
	ids, err := h.PlanQ.Pop(r.Context(), max)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, idStr := range ids {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil && h.Store != nil {
			_ = h.Store.UpdatePendingInputStatus(r.Context(), id, "planning", "")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": ids})
}

func (h *Handler) pendingInputStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/pending-inputs/status/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var body struct {
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if body.Status == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status required"})
		return
	}
	if err := h.Store.UpdatePendingInputStatus(r.Context(), id, body.Status, body.Error); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"detail": "status updated"})
}

func (h *Handler) planQueueClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if h.PlanQ == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "plan queue not configured"})
		return
	}
	ids, err := h.PlanQ.Clear(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	reset := 0
	if h.Store != nil {
		for _, idStr := range ids {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			if err := h.Store.UpdatePendingInputStatus(r.Context(), id, "pending", ""); err == nil {
				reset++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"detail":       "plan queue cleared",
		"cleared":      len(ids),
		"status_reset": reset,
	})
}

func (h *Handler) builds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		limit := parseIntDefault(r.URL.Query().Get("limit"), 200, 1000)
		list, err := h.Store.ListBuilds(r.Context(), status, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodDelete:
		if err := h.requireWorkerToken(r); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		status := r.URL.Query().Get("status")
		count, err := h.Store.DeleteBuilds(r.Context(), status)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"detail": "builds cleared", "count": count})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) buildStatusUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		Package      string `json:"package"`
		Version      string `json:"version"`
		Status       string `json:"status"`
		Error        string `json:"error,omitempty"`
		Attempts     int    `json:"attempts,omitempty"`
		BackoffUntil int64  `json:"backoff_until,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if body.Package == "" || body.Version == "" || body.Status == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "package, version, and status required"})
		return
	}
	if err := h.Store.UpdateBuildStatus(r.Context(), body.Package, body.Version, body.Status, body.Error, body.Attempts, body.BackoffUntil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"detail": "build status updated"})
}

func (h *Handler) buildQueuePop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.requireWorkerToken(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	max := parseIntDefault(r.URL.Query().Get("max"), 5, 100)
	builds, err := h.Store.LeaseBuilds(r.Context(), max)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type job struct {
		Package     string `json:"package"`
		Version     string `json:"version"`
		PythonTag   string `json:"python_tag"`
		PlatformTag string `json:"platform_tag"`
		Attempts    int    `json:"attempts"`
		RunID       string `json:"run_id,omitempty"`
		PlanID      int64  `json:"plan_id,omitempty"`
	}
	var out []job
	for _, b := range builds {
		out = append(out, job{
			Package:     b.Package,
			Version:     b.Version,
			PythonTag:   b.PythonTag,
			PlatformTag: b.PlatformTag,
			Attempts:    b.Attempts,
			RunID:       b.RunID,
			PlanID:      b.PlanID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"builds": out})
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
			RunID          string           `json:"run_id"`
			Plan           []store.PlanNode `json:"plan"`
			DAG            json.RawMessage  `json:"dag,omitempty"`
			PendingInputID int64            `json:"pending_input_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if len(body.Plan) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan required"})
			return
		}
		planID, err := h.Store.SavePlan(r.Context(), body.RunID, body.Plan, body.DAG)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if body.PendingInputID > 0 && h.Store != nil {
			if err := h.Store.LinkPlanToPendingInput(r.Context(), body.PendingInputID, planID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			_ = h.Store.UpdatePendingInputStatus(r.Context(), body.PendingInputID, "planned", "")
		}
		if h.Config.AutoBuild {
			_ = h.Store.QueueBuildsFromPlan(r.Context(), body.RunID, planID, body.Plan)
			if body.PendingInputID > 0 {
				_ = h.Store.UpdatePendingInputStatus(r.Context(), body.PendingInputID, "queued", "")
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"detail": "plan saved"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) plans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := parseIntDefault(r.URL.Query().Get("limit"), 20, 200)
		list, err := h.Store.ListPlans(r.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodDelete:
		if err := h.requireWorkerToken(r); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if h.Store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not configured"})
			return
		}
		var planID int64
		if idStr := r.URL.Query().Get("id"); idStr != "" {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || id <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
				return
			}
			planID = id
		}
		if h.Store != nil {
			_, _ = h.Store.UpdatePendingInputsForPlan(r.Context(), planID, "pending")
		}
		count, err := h.Store.DeletePlans(r.Context(), planID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"detail": "plans cleared", "count": count})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) planLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	snap, err := h.Store.LatestPlanSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (h *Handler) planByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/plan/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan id required"})
		return
	}
	planID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid plan id"})
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch r.Method {
	case http.MethodGet:
		if action != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		snap, err := h.Store.PlanSnapshot(r.Context(), planID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, snap)
	case http.MethodPost:
		if action != "enqueue-builds" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		snap, err := h.Store.PlanSnapshot(r.Context(), planID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := h.Store.QueueBuildsFromPlan(r.Context(), snap.RunID, snap.ID, snap.Plan); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if h.Store != nil {
			_, _ = h.Store.UpdatePendingInputsForPlan(r.Context(), snap.ID, "queued")
		}
		count := 0
		for _, node := range snap.Plan {
			if strings.EqualFold(node.Action, "build") {
				count++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"detail":   "builds enqueued",
			"plan_id":  snap.ID,
			"run_id":   snap.RunID,
			"enqueued": count,
		})
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
		runID := toString(snap["run_id"])
		var dagRaw json.RawMessage
		if dag, ok := snap["dag"]; ok {
			if data, err := json.Marshal(dag); err == nil {
				dagRaw = data
			}
		}
		planID, _ := h.Store.SavePlan(ctx, runID, nodes, dagRaw)
		if h.Config.AutoBuild {
			_ = h.Store.QueueBuildsFromPlan(ctx, runID, planID, nodes)
		}
	}
	writeJSON(w, http.StatusOK, snap)
}

func pyVersionFromTag(tag, fallback string) string {
	if fallback != "" {
		return fallback
	}
	tag = strings.TrimPrefix(tag, "cp")
	if len(tag) == 0 {
		return ""
	}
	if len(tag) == 3 {
		return fmt.Sprintf("%s.%s", tag[:1], tag[1:])
	}
	if len(tag) == 4 {
		return fmt.Sprintf("%s.%s", tag[:2], tag[2:])
	}
	return ""
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
	ctx := r.Context()
	items, err := h.Queue.List(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	stats, _ := h.Queue.Stats(ctx)
	resp := map[string]any{
		"items":        items,
		"length":       len(items),
		"worker_mode":  h.Config.QueueBackend,
		"oldest_age_s": stats.OldestAge,
		"auto_build":   h.Config.AutoBuild,
	}
	writeJSON(w, http.StatusOK, resp)
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
		q := r.URL.Query()
		limit := parseIntDefault(q.Get("limit"), 200, 1000)
		offset := parseIntDefault(q.Get("offset"), 0, 100_000)
		query := strings.TrimSpace(q.Get("q"))
		var hints []store.Hint
		var err error
		if pager, ok := h.Store.(interface {
			ListHintsPaged(context.Context, int, int, string) ([]store.Hint, error)
		}); ok {
			hints, err = pager.ListHintsPaged(r.Context(), limit, offset, query)
		} else {
			hints, err = h.Store.ListHints(r.Context())
			if err == nil {
				if query != "" {
					filtered := make([]store.Hint, 0, len(hints))
					for _, h := range hints {
						if hintMatchesQuery(h, query) {
							filtered = append(filtered, h)
						}
					}
					hints = filtered
				}
				if offset > 0 && offset < len(hints) {
					hints = hints[offset:]
				} else if offset >= len(hints) {
					hints = []store.Hint{}
				}
				if limit > 0 && limit < len(hints) {
					hints = hints[:limit]
				}
			}
		}
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
		hint = store.NormalizeHint(hint)
		if errs := store.ValidateHint(hint); len(errs) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "validation failed",
				"details": errs,
			})
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

func (h *Handler) hintsBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.Store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not configured"})
		return
	}
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
			return
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read file"})
			return
		}
	} else {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
			return
		}
	}
	if len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty payload"})
		return
	}
	var hints []store.Hint
	if err := yaml.Unmarshal(data, &hints); err != nil {
		if err2 := json.Unmarshal(data, &hints); err2 != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid yaml/json"})
			return
		}
	}
	var loaded, skipped int
	var errors []string
	for idx, hint := range hints {
		hint = store.NormalizeHint(hint)
		if errs := store.ValidateHint(hint); len(errs) > 0 {
			skipped++
			label := hint.ID
			if label == "" {
				label = fmt.Sprintf("index %d", idx)
			}
			errors = append(errors, fmt.Sprintf("%s: %s", label, strings.Join(errs, "; ")))
			continue
		}
		if err := h.Store.PutHint(r.Context(), hint); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", hint.ID, err))
			skipped++
			continue
		}
		loaded++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"loaded":  loaded,
		"skipped": skipped,
		"errors":  errors,
	})
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
		hint = store.NormalizeHint(hint)
		if errs := store.ValidateHint(hint); len(errs) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "validation failed",
				"details": errs,
			})
			return
		}
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

func hintMatchesQuery(h store.Hint, q string) bool {
	query := strings.ToLower(strings.TrimSpace(q))
	if query == "" {
		return true
	}
	parts := []string{h.ID, h.Pattern, h.Note, h.Severity, h.Confidence}
	parts = append(parts, h.Tags...)
	parts = append(parts, h.Examples...)
	for _, recipes := range h.Recipes {
		parts = append(parts, recipes...)
	}
	for _, applies := range h.AppliesTo {
		parts = append(parts, applies...)
	}
	joined := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(joined, query)
}
