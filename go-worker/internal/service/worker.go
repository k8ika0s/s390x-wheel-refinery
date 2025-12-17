package service

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/builder"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/cas"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/objectstore"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/reporter"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
	"golang.org/x/sync/errgroup"
)

// Worker drains the queue and runs jobs.
type Worker struct {
	Queue    queue.Backend
	Runner   runner.Runner
	Reporter *reporter.Client
	Cfg      Config
	Store    objectstore.Store
	Fetcher  cas.Fetcher
	Pusher   cas.Pusher
	packPath map[string]string
	mu       sync.Mutex
	planSnap plan.Snapshot
}

type result struct {
	job      runner.Job
	duration time.Duration
	log      string
	err      error
	repair   artifact.ID
	attempt  int
}

// LoadPlan reads plan.json if present.
func (w *Worker) LoadPlan() error {
	path := filepath.Join(w.Cfg.OutputDir, "plan.json")
	snap, err := plan.Load(path)
	if err != nil {
		// fall back to cache plan
		path = filepath.Join(w.Cfg.CacheDir, "plan.json")
		snap, err = plan.Load(path)
		if err != nil {
			// as a last resort, generate plan in Go
			snap, err = plan.Generate(
				w.Cfg.InputDir,
				w.Cfg.CacheDir,
				w.Cfg.PythonVersion,
				w.Cfg.PlatformTag,
				w.Cfg.IndexURL,
				w.Cfg.ExtraIndexURL,
				w.Cfg.UpgradeStrategy,
				w.Cfg.RequirementsPath,
				w.Cfg.ConstraintsPath,
				w.Cfg.PackCatalog,
				w.Cfg.CASStore(),
				w.Cfg.CASRegistryURL,
				w.Cfg.CASRegistryRepo,
			)
			if err != nil {
				return err
			}
		}
	}
	w.mu.Lock()
	w.planSnap = snap
	w.mu.Unlock()
	return nil
}

// Drain pops from queue and executes matched jobs.
func (w *Worker) Drain(ctx context.Context) error {
	if err := w.LoadPlan(); err != nil {
		return fmt.Errorf("load plan: %w", err)
	}
	var reqs []queue.Request
	var err error
	if w.Cfg.BuildPopURL != "" {
		reqs, err = w.popBuildQueue(ctx)
	} else {
		reqs, err = w.Queue.Pop(ctx, w.Cfg.BatchSize)
	}
	if err != nil {
		return err
	}
	if len(reqs) == 0 {
		return nil
	}

	reqAttempts := make(map[string]int)
	for _, r := range reqs {
		key := queueKey(r.Package, r.Version)
		if r.Attempts > reqAttempts[key] {
			reqAttempts[key] = r.Attempts
		}
	}

	jobs := w.match(ctx, reqs)
	results := make([]result, len(jobs))
	g, ctx := errgroup.WithContext(ctx)
	for i, job := range jobs {
		i, job := i, job
		g.Go(func() error {
			w.reportBuildStatus(ctx, job.Name, job.Version, "building", nil, reqAttempts[queueKey(job.Name, job.Version)], 0)
			if job.WheelAction == "reuse" && job.WheelDigest != "" {
				if err := w.fetchWheel(ctx, job); err != nil {
					return fmt.Errorf("fetch wheel %s: %w", job.WheelDigest, err)
				}
			}
			dur, logContent, err := w.Runner.Run(ctx, job)
			repID := artifact.ID{}
			results[i] = result{
				job:      job,
				duration: dur,
				log:      logContent,
				err:      err,
				repair:   repID,
				attempt:  reqAttempts[queueKey(job.Name, job.Version)],
			}
			if err == nil && w.Cfg.RepairPushEnabled && job.WheelDigest != "" && w.Pusher.BaseURL != "" {
				repKey := artifact.RepairKey{InputWheelDigest: job.WheelDigest}
				repID = artifact.ID{Type: artifact.RepairType, Digest: repKey.Digest()}
				results[i].repair = repID
			}
			return nil
		})
	}
	_ = g.Wait()

	var manifestEntries []map[string]any
	var firstErr error
	for _, res := range results {
		status := "built"
		meta := map[string]any{"duration_ms": res.duration.Milliseconds()}
		if res.err != nil {
			status = "failed"
			meta["error"] = res.err.Error()
			if firstErr == nil {
				firstErr = res.err
			}
			if w.shouldRequeue(reqAttempts, res.job) {
				status = "retry"
			}
		}
		// report build status to control-plane
		backoffUntil := int64(0)
		if status == "retry" {
			backoffUntil = backoffTime(res.attempt)
		}
		w.reportBuildStatus(ctx, res.job.Name, res.job.Version, status, res.err, res.attempt, backoffUntil)
		if res.job.WheelDigest != "" {
			meta["wheel_digest"] = res.job.WheelDigest
			if res.job.WheelSourceDigest != "" {
				meta["wheel_source_digest"] = res.job.WheelSourceDigest
			}
			if u := w.casURL(artifact.ID{Type: artifact.WheelType, Digest: res.job.WheelDigest}); u != "" {
				meta["wheel_url"] = u
			} else if res.job.WheelDigest != "" {
				if u := w.objectURL(res.job, "wheel"); u != "" {
					meta["wheel_url"] = u
				}
			}
		}
		if res.job.WheelAction != "" {
			meta["wheel_action"] = res.job.WheelAction
		}
		if res.job.RuntimeDigest != "" {
			meta["runtime_digest"] = res.job.RuntimeDigest
			if u := w.casURL(artifact.ID{Type: artifact.RuntimeType, Digest: res.job.RuntimeDigest}); u != "" {
				meta["runtime_url"] = u
			}
		}
		if len(res.job.PackDigests) > 0 {
			meta["pack_digests"] = res.job.PackDigests
			var urls []string
			for _, d := range res.job.PackDigests {
				if u := w.casURL(artifact.ID{Type: artifact.PackType, Digest: d}); u != "" {
					urls = append(urls, u)
				}
			}
			if len(urls) > 0 {
				meta["pack_urls"] = urls
			}
		}
		if res.repair.Type == artifact.RepairType && res.repair.Digest != "" {
			meta["repair_digest"] = res.repair.Digest
			if w.Cfg.RepairToolVersion != "" {
				meta["repair_tool_version"] = w.Cfg.RepairToolVersion
			}
			if w.Cfg.RepairPolicyHash != "" {
				meta["repair_policy_hash"] = w.Cfg.RepairPolicyHash
			}
			if u := w.casURL(res.repair); u != "" {
				meta["repair_url"] = u
			} else if u := w.objectURL(res.job, "repair"); u != "" {
				meta["repair_url"] = u
			}
		}

		wheelURL := ""
		if v, ok := meta["wheel_url"].(string); ok {
			wheelURL = v
		}
		repairURL := ""
		if v, ok := meta["repair_url"].(string); ok {
			repairURL = v
		}
		repairDigest := ""
		if v, ok := meta["repair_digest"].(string); ok {
			repairDigest = v
		}
		entry := map[string]any{
			"name":          res.job.Name,
			"version":       res.job.Version,
			"status":        status,
			"python_tag":    res.job.PythonTag,
			"platform_tag":  res.job.PlatformTag,
			"wheel":         wheelURL,
			"repair_url":    repairURL,
			"repair_digest": repairDigest,
			"metadata":      meta,
		}
		manifestEntries = append(manifestEntries, entry)

		logPayload := map[string]any{
			"name":        res.job.Name,
			"version":     res.job.Version,
			"status":      status,
			"duration_ms": res.duration.Milliseconds(),
			"content":     res.log,
		}
		if res.err != nil {
			logPayload["error"] = res.err.Error()
		}
		if res.job.WheelDigest != "" {
			logPayload["wheel_digest"] = res.job.WheelDigest
			if res.job.WheelSourceDigest != "" {
				logPayload["wheel_source_digest"] = res.job.WheelSourceDigest
			}
			if u := w.casURL(artifact.ID{Type: artifact.WheelType, Digest: res.job.WheelDigest}); u != "" {
				logPayload["wheel_url"] = u
			} else if u := w.objectURL(res.job, "wheel"); u != "" {
				logPayload["wheel_url"] = u
			}
		}
		if res.job.WheelAction != "" {
			logPayload["wheel_action"] = res.job.WheelAction
		}
		if res.job.RuntimeDigest != "" {
			logPayload["runtime_digest"] = res.job.RuntimeDigest
			if u := w.casURL(artifact.ID{Type: artifact.RuntimeType, Digest: res.job.RuntimeDigest}); u != "" {
				logPayload["runtime_url"] = u
			}
		}
		if len(res.job.PackDigests) > 0 {
			logPayload["pack_digests"] = res.job.PackDigests
			var urls []string
			for _, d := range res.job.PackDigests {
				if u := w.casURL(artifact.ID{Type: artifact.PackType, Digest: d}); u != "" {
					urls = append(urls, u)
				}
			}
			if len(urls) > 0 {
				logPayload["pack_urls"] = urls
			}
		}
		if res.repair.Type == artifact.RepairType && res.repair.Digest != "" {
			logPayload["repair_digest"] = res.repair.Digest
			if w.Cfg.RepairToolVersion != "" {
				logPayload["repair_tool_version"] = w.Cfg.RepairToolVersion
			}
			if w.Cfg.RepairPolicyHash != "" {
				logPayload["repair_policy_hash"] = w.Cfg.RepairPolicyHash
			}
			if u := w.casURL(res.repair); u != "" {
				logPayload["repair_url"] = u
			} else if u := w.objectURL(res.job, "repair"); u != "" {
				logPayload["repair_url"] = u
			}
		}
		if w.Reporter != nil {
			_ = w.Reporter.PostLog(logPayload)
			_ = w.Reporter.PostManifest([]map[string]any{entry})
			_ = w.Reporter.PostEvent(map[string]any{
				"name":         res.job.Name,
				"version":      res.job.Version,
				"python_tag":   res.job.PythonTag,
				"platform_tag": res.job.PlatformTag,
				"status":       status,
				"timestamp":    time.Now().Unix(),
				"metadata":     meta,
			})
		}

		if res.err == nil {
			w.uploadArtifacts(ctx, res.job)
		}
	}

	if len(manifestEntries) > 0 {
		writeManifest(w.Cfg.OutputDir, manifestEntries)
	}
	return firstErr
}

func (w *Worker) reportBuildStatus(ctx context.Context, pkg, version, status string, err error, attempts int, backoffUntil int64) {
	if w.Cfg.ControlPlaneURL == "" {
		return
	}
	url := strings.TrimRight(w.Cfg.ControlPlaneURL, "/") + "/api/builds/status"
	body := map[string]any{
		"package":  pkg,
		"version":  version,
		"status":   status,
		"attempts": attempts,
	}
	if err != nil {
		body["error"] = err.Error()
	}
	if backoffUntil > 0 {
		body["backoff_until"] = backoffUntil
	}
	data, _ := json.Marshal(body)
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if reqErr != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if w.Cfg.ControlPlaneToken != "" {
		req.Header.Set("X-Worker-Token", w.Cfg.ControlPlaneToken)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, doErr := client.Do(req)
	if doErr != nil {
		return
	}
	resp.Body.Close()
}

func (w *Worker) match(ctx context.Context, reqs []queue.Request) []runner.Job {
	w.mu.Lock()
	snap := w.planSnap
	w.mu.Unlock()

	packActions := map[string]string{}
	packMeta := map[string]map[string]any{}
	runtimeActions := map[string]string{}
	runtimeMeta := map[string]map[string]any{}
	for _, n := range snap.DAG {
		switch n.Type {
		case plan.NodePack:
			packActions[n.ID.Digest] = n.Action
			packMeta[n.ID.Digest] = n.Metadata
		case plan.NodeRuntime:
			runtimeActions[n.ID.Digest] = n.Action
			runtimeMeta[n.ID.Digest] = n.Metadata
		}
	}

	var jobs []runner.Job
	for _, req := range reqs {
		for _, node := range snap.Plan {
			if node.Name == "" || node.Version == "" {
				continue
			}
			if !equalsIgnoreCase(node.Name, req.Package) {
				continue
			}
			if req.Version != "" && req.Version != "latest" && req.Version != node.Version {
				continue
			}
			wheelDigest, wheelAction, packIDs, runtimeID := findWheelArtifact(snap.DAG, node, req)
			orderedPacks := topoSortFromDag(packIDs, snap.DAG)
			jobs = append(jobs, runner.Job{
				Name:              node.Name,
				Version:           node.Version,
				PythonVersion:     firstNonEmpty(req.PythonVersion, node.PythonVersion),
				PythonTag:         firstNonEmpty(node.PythonTag, pyTagFromVersion(firstNonEmpty(req.PythonVersion, node.PythonVersion))),
				PlatformTag:       node.PlatformTag,
				Recipes:           req.Recipes,
				WheelDigest:       wheelDigest,
				WheelAction:       wheelAction,
				WheelSourceDigest: findWheelSourceDigest(snap.DAG, wheelDigest),
				RepairToolVersion: findRepairToolVersion(snap.DAG, wheelDigest),
				RepairPolicyHash:  findRepairPolicyHash(snap.DAG, wheelDigest),
				PackPaths:         w.resolvePacks(ctx, orderedPacks, packActions, packMeta),
				RuntimePath:       w.fetchRuntime(ctx, firstNonEmpty(req.PythonVersion, node.PythonVersion), runtimeID, runtimeActions[runtimeID.Digest], runtimeMeta[runtimeID.Digest]),
				RuntimeDigest:     runtimeID.Digest,
				PackDigests:       packDigests(orderedPacks),
			})
		}
	}
	return jobs
}

func equalsIgnoreCase(a, b string) bool {
	return strings.EqualFold(a, b)
}

func findWheelArtifact(dag []plan.DAGNode, node plan.FlatNode, req queue.Request) (digest, action string, packIDs []artifact.ID, runtimeID artifact.ID) {
	for _, n := range dag {
		if n.Type != plan.NodeWheel {
			continue
		}
		name, _ := n.Metadata["name"].(string)
		ver, _ := n.Metadata["version"].(string)
		if !equalsIgnoreCase(name, node.Name) || ver != node.Version {
			continue
		}
		pyTag, _ := n.Metadata["python_tag"].(string)
		platTag, _ := n.Metadata["platform_tag"].(string)
		if pyTag != "" && pyTag != node.PythonTag && pyTag != req.PythonTag {
			continue
		}
		if platTag != "" && platTag != node.PlatformTag && platTag != req.PlatformTag {
			continue
		}
		var packs []artifact.ID
		for _, inp := range n.Inputs {
			if inp.Type == artifact.PackType {
				packs = append(packs, inp)
			}
			if inp.Type == artifact.RuntimeType {
				runtimeID = inp
			}
		}
		return n.ID.Digest, n.Action, packs, runtimeID
	}
	return "", "", nil, artifact.ID{}
}

func packDigests(ids []artifact.ID) []string {
	var out []string
	for _, id := range ids {
		if id.Type == artifact.PackType {
			out = append(out, id.Digest)
		}
	}
	return out
}

func findWheelSourceDigest(dag []plan.DAGNode, wheelDigest string) string {
	for _, n := range dag {
		if n.Type != plan.NodeWheel || n.ID.Digest != wheelDigest {
			continue
		}
		if sd, ok := n.Metadata["source_digest"].(string); ok {
			return sd
		}
	}
	return ""
}

func findRepairToolVersion(dag []plan.DAGNode, wheelDigest string) string {
	for _, n := range dag {
		if n.Type != plan.NodeRepair {
			continue
		}
		if len(n.Inputs) == 0 || n.Inputs[0].Digest != wheelDigest {
			continue
		}
		if v, ok := n.Metadata["repair_tool_version"].(string); ok {
			return v
		}
	}
	return ""
}

func findRepairPolicyHash(dag []plan.DAGNode, wheelDigest string) string {
	for _, n := range dag {
		if n.Type != plan.NodeRepair {
			continue
		}
		if len(n.Inputs) == 0 || n.Inputs[0].Digest != wheelDigest {
			continue
		}
		if v, ok := n.Metadata["repair_policy_digest"].(string); ok {
			return v
		}
	}
	return ""
}

func (w *Worker) casURL(id artifact.ID) string {
	if w.Cfg.CASRegistryURL == "" || id.Digest == "" {
		return ""
	}
	repo := w.Cfg.CASRegistryRepo
	if repo == "" {
		repo = "artifacts"
	}
	return fmt.Sprintf("%s/v2/%s/blobs/%s", strings.TrimRight(w.Cfg.CASRegistryURL, "/"), strings.Trim(repo, "/"), id.Digest)
}

func (w *Worker) objectURL(job runner.Job, kind string) string {
	if w.Store == nil {
		return ""
	}
	base := fmt.Sprintf("%s/%s/", strings.ToLower(job.Name), job.Version)
	key := base
	if kind == "repair" && job.WheelDigest != "" {
		key = fmt.Sprintf("%srepair-%s", base, job.WheelDigest)
	}
	if os, ok := w.Store.(interface{ URL(string) string }); ok {
		return os.URL(key)
	}
	return ""
}

// requestsFromPlan seeds work items directly from the current plan when the queue is empty.
func (w *Worker) requestsFromPlan() []queue.Request {
	// Legacy helper retained for backward compatibility; unused in Drain.
	w.mu.Lock()
	snap := w.planSnap
	w.mu.Unlock()
	reqs := []queue.Request{}
	now := time.Now().Unix()
	for _, node := range snap.Plan {
		if strings.ToLower(node.Action) != "build" {
			continue
		}
		if node.Name == "" || node.Version == "" {
			continue
		}
		reqs = append(reqs, queue.Request{
			Package:       node.Name,
			Version:       node.Version,
			PythonVersion: node.PythonVersion,
			PythonTag:     node.PythonTag,
			PlatformTag:   node.PlatformTag,
			Attempts:      0,
			EnqueuedAt:    now,
		})
	}
	return reqs
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func pyTagFromVersion(ver string) string {
	if ver == "" {
		return ""
	}
	trimmed := strings.ReplaceAll(ver, ".", "")
	if strings.HasPrefix(trimmed, "cp") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "3") {
		return "cp" + trimmed
	}
	return trimmed
}

func pyVersionFromTag(tag string) string {
	tag = strings.TrimPrefix(tag, "cp")
	if len(tag) == 3 {
		return fmt.Sprintf("%s.%s", tag[:1], tag[1:])
	}
	if len(tag) == 4 {
		return fmt.Sprintf("%s.%s", tag[:2], tag[2:])
	}
	return ""
}

// BuildWorker constructs a worker from config.
func BuildWorker(cfg Config) (*Worker, error) {
	if cfg.BuildPopURL == "" && cfg.ControlPlaneURL != "" {
		cfg.BuildPopURL = strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/build-queue/pop"
	}
	var q queue.Backend
	switch cfg.QueueBackend {
	case "redis":
		q = queue.NewRedisQueue(cfg.RedisURL, cfg.RedisKey)
	case "kafka":
		q = queue.NewKafkaQueue(cfg.KafkaBrokers, cfg.KafkaTopic)
	default:
		q = queue.NewFileQueue(cfg.QueueFile)
	}
	if q == nil {
		return nil, errors.New("queue backend not configured")
	}
	r := &runner.PodmanRunner{
		Image:       cfg.ContainerImage,
		InputDir:    cfg.InputDir,
		OutputDir:   cfg.OutputDir,
		CacheDir:    cfg.CacheDir,
		PythonTag:   cfg.PythonVersion,
		PlatformTag: cfg.PlatformTag,
		Bin:         cfg.PodmanBin,
		Timeout:     time.Duration(cfg.RunnerTimeoutSec) * time.Second,
		RunCmd:      cfg.RunCmd,
	}
	rep := &reporter.Client{BaseURL: strings.TrimRight(cfg.ControlPlaneURL, "/"), Token: cfg.ControlPlaneToken}
	return &Worker{
		Queue:    q,
		Runner:   r,
		Reporter: rep,
		Cfg:      cfg,
		Store:    cfg.ObjectStore(),
		Fetcher:  cfg.CASFetcher(),
		Pusher: cas.Pusher{
			BaseURL:  cfg.CASRegistryURL,
			Repo:     cfg.CASRegistryRepo,
			Username: cfg.CASRegistryUser,
			Password: cfg.CASRegistryPass,
		},
		packPath: make(map[string]string),
	}, nil
}

func queueKey(name, version string) string {
	return strings.ToLower(name) + "::" + strings.ToLower(version)
}

func (w *Worker) shouldRequeue(reqAttempts map[string]int, job runner.Job) bool {
	if !w.Cfg.RequeueOnFailure {
		return false
	}
	key := queueKey(job.Name, job.Version)
	attempt := reqAttempts[key]
	if attempt >= w.Cfg.MaxRequeueAttempts {
		return false
	}
	return true
}

// popBuildQueue pulls ready builds from control-plane build queue (if configured).
func (w *Worker) popBuildQueue(ctx context.Context) ([]queue.Request, error) {
	url := w.Cfg.BuildPopURL
	if url == "" && w.Cfg.ControlPlaneURL != "" {
		url = strings.TrimRight(w.Cfg.ControlPlaneURL, "/") + "/api/build-queue/pop"
	}
	if url == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	if w.Cfg.BatchSize > 0 {
		q := req.URL.Query()
		q.Set("max", strconv.Itoa(w.Cfg.BatchSize))
		req.URL.RawQuery = q.Encode()
	}
	if w.Cfg.ControlPlaneToken != "" {
		req.Header.Set("X-Worker-Token", w.Cfg.ControlPlaneToken)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("build queue pop: status %d: %s", resp.StatusCode, string(b))
	}
	var payload struct {
		Builds []struct {
			Package     string `json:"package"`
			Version     string `json:"version"`
			PythonTag   string `json:"python_tag"`
			PlatformTag string `json:"platform_tag"`
			Attempts    int    `json:"attempts"`
		} `json:"builds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var out []queue.Request
	for _, b := range payload.Builds {
		out = append(out, queue.Request{
			Package:       b.Package,
			Version:       b.Version,
			PythonTag:     b.PythonTag,
			PythonVersion: pyVersionFromTag(b.PythonTag),
			PlatformTag:   b.PlatformTag,
			Attempts:      b.Attempts,
		})
	}
	return out, nil
}

// backoffTime returns a Unix timestamp for the next retry using capped exponential backoff with jitter.
func backoffTime(attempt int) int64 {
	if attempt < 1 {
		attempt = 1
	}
	base := 5 * time.Second
	max := 10 * time.Minute
	d := base * time.Duration(1<<(attempt-1))
	if d > max {
		d = max
	}
	// Add up to 1s jitter to avoid thundering herd.
	jitter := time.Duration(rand.Int63n(int64(time.Second)))
	return time.Now().Add(d + jitter).Unix()
}

// writeManifest writes manifest.json locally (best effort).
func writeManifest(outputDir string, manifest any) {
	if outputDir == "" {
		return
	}
	path := filepath.Join(outputDir, "manifest.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
}

// RunOnce is used by trigger handler.
func (w *Worker) RunOnce(ctx context.Context) error {
	return w.Drain(ctx)
}

// uploadArtifacts pushes built wheel files to object storage (best effort).
func (w *Worker) uploadArtifacts(ctx context.Context, job runner.Job) {
	store := w.Store
	if store == nil {
		return
	}
	entries, err := os.ReadDir(w.Cfg.OutputDir)
	if err != nil {
		return
	}
	// Pack publish is not tied to specific files; packs are metadata-only here.
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".whl") {
			continue
		}
		// crude match: wheel filename contains package name
		if !strings.Contains(strings.ToLower(e.Name()), strings.ToLower(job.Name)) {
			continue
		}
		path := filepath.Join(w.Cfg.OutputDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if job.WheelDigest != "" {
			if ok, err := verifyBytesDigest(data, job.WheelDigest); err != nil || !ok {
				log.Printf("skip CAS push for wheel: digest mismatch %s", job.WheelDigest)
				continue
			}
		}
		key := fmt.Sprintf("%s/%s/%s", strings.ToLower(job.Name), job.Version, e.Name())
		_ = store.Put(ctx, key, data, "application/octet-stream")
		if w.Cfg.CASPushEnabled && w.Pusher.BaseURL != "" && job.WheelDigest != "" {
			_, _ = w.Pusher.Push(ctx, artifact.ID{Type: artifact.WheelType, Digest: job.WheelDigest}, data, "application/octet-stream")
		}
	}
	if w.Cfg.RepairPushEnabled && w.Pusher.BaseURL != "" && job.WheelDigest != "" {
		repPath := filepath.Join(w.Cfg.OutputDir, fmt.Sprintf("%s-%s-repair.whl", job.Name, job.Version))
		repData, err := os.ReadFile(repPath)
		if err != nil {
			if wheelPath := w.wheelFileForJob(job); wheelPath != "" {
				_ = w.runRepair(wheelPath, repPath)
				repData, _ = os.ReadFile(repPath)
			} else {
				if err := w.writeStubArtifact(repPath, "repair", job.WheelDigest, map[string]any{"name": job.Name, "version": job.Version}); err == nil {
					repData, _ = os.ReadFile(repPath)
				}
			}
		}
		if len(repData) > 0 {
			repKey := artifact.RepairKey{
				InputWheelDigest:  job.WheelDigest,
				RepairToolVersion: w.Cfg.RepairToolVersion,
				PolicyRulesDigest: w.Cfg.RepairPolicyHash,
			}
			if ok, err := verifyBytesDigest(repData, repKey.Digest()); err == nil && !ok {
				log.Printf("skip CAS push for repair: digest mismatch %s", repKey.Digest())
			} else {
				_, _ = w.Pusher.Push(ctx, artifact.ID{Type: artifact.RepairType, Digest: repKey.Digest()}, repData, "application/octet-stream")
				if store != nil {
					repairKey := fmt.Sprintf("%s/%s/repair-%s.whl", strings.ToLower(job.Name), job.Version, job.WheelDigest)
					_ = store.Put(ctx, repairKey, repData, "application/octet-stream")
				}
			}
		}
	}
	// Optional pack/runtime publish (empty placeholder payload)
	if w.Cfg.PackPushEnabled && len(job.PackDigests) > 0 {
		for idx, d := range job.PackDigests {
			if idx < len(job.PackPaths) && job.PackPaths[idx] != "" {
				if data, err := os.ReadFile(job.PackPaths[idx]); err == nil {
					if ok, err := verifyBytesDigest(data, d); err == nil && !ok {
						log.Printf("skip CAS push for pack: digest mismatch %s", d)
						continue
					}
					_, _ = w.Pusher.Push(ctx, artifact.ID{Type: artifact.PackType, Digest: d}, data, "application/octet-stream")
					continue
				}
			}
			if stub, err := w.stubPayload("pack", d, nil); err == nil {
				_, _ = w.Pusher.Push(ctx, artifact.ID{Type: artifact.PackType, Digest: d}, stub, "application/octet-stream")
			}
		}
	}
	if w.Cfg.RuntimePushEnabled && job.RuntimeDigest != "" {
		if job.RuntimePath != "" {
			if data, err := os.ReadFile(job.RuntimePath); err == nil {
				if ok, err := verifyBytesDigest(data, job.RuntimeDigest); err == nil && !ok {
					log.Printf("skip CAS push for runtime: digest mismatch %s", job.RuntimeDigest)
					return
				}
				_, _ = w.Pusher.Push(ctx, artifact.ID{Type: artifact.RuntimeType, Digest: job.RuntimeDigest}, data, "application/octet-stream")
				return
			}
		}
		if stub, err := w.stubPayload("runtime", job.RuntimeDigest, nil); err == nil {
			_, _ = w.Pusher.Push(ctx, artifact.ID{Type: artifact.RuntimeType, Digest: job.RuntimeDigest}, stub, "application/octet-stream")
		}
	}
}

func (w *Worker) stubPayload(kind, digest string, meta map[string]any) ([]byte, error) {
	data := map[string]any{
		"kind":   kind,
		"digest": digest,
	}
	for k, v := range meta {
		data[k] = v
	}
	return json.MarshalIndent(data, "", "  ")
}

func (w *Worker) wheelFileForJob(job runner.Job) string {
	entries, err := os.ReadDir(w.Cfg.OutputDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".whl") {
			continue
		}
		if strings.Contains(strings.ToLower(e.Name()), strings.ToLower(job.Name)) {
			return filepath.Join(w.Cfg.OutputDir, e.Name())
		}
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeTarWithManifest(path string, manifest map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	hdr := &tar.Header{
		Name:    "manifest.json",
		Mode:    0o644,
		Size:    int64(len(payload)),
		ModTime: time.Unix(0, 0),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(payload); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func extractTar(src, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		default:
			// skip other types for now
		}
	}
	return nil
}

func verifyFileDigest(path, expected string) (bool, error) {
	if expected == "" || !strings.HasPrefix(expected, "sha256:") {
		return true, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return verifyBytesDigest(data, expected)
}

func verifyBytesDigest(data []byte, expected string) (bool, error) {
	if expected == "" || !strings.HasPrefix(expected, "sha256:") {
		return true, nil
	}
	sum := sha256.Sum256(data)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	return actual == expected, nil
}

func (w *Worker) runRepair(wheelPath, repairPath string) error {
	if wheelPath == "" {
		return fmt.Errorf("wheel path missing for repair")
	}
	cmdStr := w.Cfg.RepairCmd
	if cmdStr == "" {
		cmdStr = w.Cfg.DefaultRepairCmd
	}
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Env = append(os.Environ(),
		"WHEEL_PATH="+wheelPath,
		"REPAIR_OUTPUT="+repairPath,
		"REPAIR_TOOL_VERSION="+w.Cfg.RepairToolVersion,
		"REPAIR_POLICY_HASH="+w.Cfg.RepairPolicyHash,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if _, err := os.Stat(repairPath); err != nil {
		return fmt.Errorf("repair output not produced: %v", err)
	}
	return nil
}

func (w *Worker) fetchWheel(ctx context.Context, job runner.Job) error {
	if job.WheelDigest == "" || w.Fetcher.BaseURL == "" {
		return nil
	}
	destDir := w.Cfg.LocalCASDir
	if destDir == "" {
		destDir = filepath.Join(w.Cfg.CacheDir, "cas")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	destPath := filepath.Join(destDir, strings.ReplaceAll(job.WheelDigest, ":", "_")+".bin")
	if err := w.Fetcher.Fetch(ctx, artifact.ID{Type: artifact.WheelType, Digest: job.WheelDigest}, destPath); err != nil {
		return err
	}
	if _, err := os.Stat(destPath); err != nil {
		return err
	}
	if ok, err := verifyFileDigest(destPath, job.WheelDigest); err != nil || !ok {
		if err != nil {
			return err
		}
		return fmt.Errorf("wheel digest mismatch: expected %s", job.WheelDigest)
	}
	return nil
}

func (w *Worker) resolvePacks(ctx context.Context, ids []artifact.ID, actions map[string]string, meta map[string]map[string]any) []string {
	if len(ids) == 0 {
		return nil
	}
	ids = sortPacksByPriority(ids, meta)
	var paths []string
	var depsBuilt []string
	for _, id := range ids {
		if id.Type != artifact.PackType {
			continue
		}
		if p, ok := w.packPath[id.Digest]; ok {
			paths = append(paths, p)
			continue
		}
		destDir := w.Cfg.LocalCASDir
		if destDir == "" {
			destDir = filepath.Join(w.Cfg.CacheDir, "cas")
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			continue
		}
		destPath := filepath.Join(destDir, strings.ReplaceAll(id.Digest, ":", "_")+".tar")
		extractDir := filepath.Join(destDir, strings.ReplaceAll(id.Digest, ":", "_"))
		fetched := false
		if w.Fetcher.BaseURL != "" {
			if err := w.Fetcher.Fetch(ctx, id, destPath); err == nil {
				if _, err := os.Stat(destPath); err == nil {
					if ok, err := verifyFileDigest(destPath, id.Digest); err == nil && ok {
						fetched = true
					}
				}
			}
		}
		if !fetched && actions[id.Digest] == "build" {
			cmd := w.Cfg.PackBuilderCmd
			if cmd == "" {
				// Derive script name from metadata name if present; fallback to default.
				if m := meta[id.Digest]; m != nil {
					if n, ok := m["name"].(string); ok && n != "" {
						script := filepath.Join(w.Cfg.PackRecipesDir, fmt.Sprintf("%s.sh", n))
						if _, err := os.Stat(script); err == nil {
							cmd = script
						}
					}
				}
				if cmd == "" {
					cmd = w.Cfg.DefaultPackCmd
				}
			}
			depsPrefixes := strings.Join(depPrefixes(paths, depsBuilt), ":")
			envCmd := cmd
			if depsPrefixes != "" {
				envCmd = fmt.Sprintf("DEPS_PREFIXES=%s %s", depsPrefixes, cmd)
			}
			if err := builder.BuildPack(destPath, builder.PackBuildOpts{Digest: id.Digest, Meta: meta[id.Digest], Cmd: envCmd}); err == nil {
				fetched = true
				depsBuilt = append(depsBuilt, destPath)
			}
		}
		if fetched {
			if err := extractTar(destPath, extractDir); err != nil {
				continue
			}
			w.packPath[id.Digest] = extractDir
			paths = append(paths, extractDir)
		}
	}
	return paths
}

func (w *Worker) fetchRuntime(ctx context.Context, pythonVersion string, rtID artifact.ID, action string, meta map[string]any) string {
	if pythonVersion == "" || rtID.Digest == "" {
		return ""
	}
	destDir := w.Cfg.LocalCASDir
	if destDir == "" {
		destDir = filepath.Join(w.Cfg.CacheDir, "cas")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return ""
	}
	destPath := filepath.Join(destDir, strings.ReplaceAll(rtID.Digest, ":", "_")+".tar")
	extractDir := filepath.Join(destDir, strings.ReplaceAll(rtID.Digest, ":", "_"))
	if w.Fetcher.BaseURL != "" {
		if err := w.Fetcher.Fetch(ctx, rtID, destPath); err == nil {
			if _, err := os.Stat(destPath); err == nil {
				if ok, err := verifyFileDigest(destPath, rtID.Digest); err == nil && ok {
					if err := extractTar(destPath, extractDir); err == nil && !isManifestOnly(extractDir) {
						return extractDir
					}
				}
			}
		}
	}
	if action == "build" {
		cmd := w.Cfg.RuntimeBuilderCmd
		if cmd == "" {
			cmd = w.Cfg.DefaultRuntimeCmd
		}
		if err := builder.BuildRuntime(destPath, builder.RuntimeBuildOpts{Digest: rtID.Digest, PythonVersion: pythonVersion, Meta: meta, Cmd: cmd}); err == nil {
			if err := extractTar(destPath, extractDir); err == nil {
				if !isManifestOnly(extractDir) || action == "build" {
					return extractDir
				}
			}
		}
	}
	return ""
}

func (w *Worker) writeStubArtifact(path, kind, digest string, meta map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := map[string]any{
		"kind":         kind,
		"digest":       digest,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range meta {
		data[k] = v
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (w *Worker) writePackArtifact(path, digest string, meta map[string]any) error {
	return writeTarWithManifest(path, map[string]any{
		"kind":         "pack",
		"digest":       digest,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"meta":         meta,
	})
}

func (w *Worker) writeRuntimeArtifact(path, digest string, meta map[string]any) error {
	return writeTarWithManifest(path, map[string]any{
		"kind":         "runtime",
		"digest":       digest,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"meta":         meta,
	})
}

func depPrefixes(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		for _, p := range g {
			if p == "" {
				continue
			}
			out = append(out, filepath.Join(p, "usr", "local"))
		}
	}
	return out
}

func sortPacksByPriority(ids []artifact.ID, meta map[string]map[string]any) []artifact.ID {
	order := map[string]int{
		"pkgconf":  1,
		"zlib":     2,
		"xz":       3,
		"bzip2":    4,
		"zstd":     5,
		"openssl":  6,
		"libffi":   7,
		"sqlite":   8,
		"libxml2":  9,
		"libxslt":  10,
		"libpng":   11,
		"jpeg":     12,
		"freetype": 13,
		"openblas": 14,
		"rust":     15,
		"cmake":    16,
		"ninja":    17,
	}
	sort.Slice(ids, func(i, j int) bool {
		mi := meta[ids[i].Digest]
		mj := meta[ids[j].Digest]
		ni, _ := mi["name"].(string)
		nj, _ := mj["name"].(string)
		return order[ni] < order[nj]
	})
	return ids
}

func isManifestOnly(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == "manifest.json" {
			continue
		}
		return false
	}
	return true
}

// topoSortFromDag orders pack IDs using DAG edges (dependencies first).
func topoSortFromDag(targets []artifact.ID, dag []plan.DAGNode) []artifact.ID {
	idSet := make(map[string]struct{})
	for _, t := range targets {
		idSet[t.Digest] = struct{}{}
	}
	nodeByDigest := make(map[string]plan.DAGNode)
	for _, n := range dag {
		if n.Type != plan.NodePack {
			continue
		}
		nodeByDigest[n.ID.Digest] = n
	}
	visited := make(map[string]bool)
	var ordered []artifact.ID
	var visit func(string)
	visit = func(d string) {
		if visited[d] {
			return
		}
		visited[d] = true
		n, ok := nodeByDigest[d]
		if ok {
			for _, inp := range n.Inputs {
				if inp.Type == artifact.PackType {
					visit(inp.Digest)
				}
			}
		}
		if _, ok := idSet[d]; ok {
			ordered = append(ordered, artifact.ID{Type: artifact.PackType, Digest: d})
		}
	}
	for d := range idSet {
		visit(d)
	}
	return ordered
}
