package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
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
	packPath map[string]string
	mu       sync.Mutex
	planSnap plan.Snapshot
}

type result struct {
	job      runner.Job
	duration time.Duration
	log      string
	err      error
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
	reqs, err := w.Queue.Pop(ctx, w.Cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(reqs) == 0 {
		reqs = w.requestsFromPlan()
		if w.Cfg.BatchSize > 0 && len(reqs) > w.Cfg.BatchSize {
			reqs = reqs[:w.Cfg.BatchSize]
		}
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
			if job.WheelAction == "reuse" && job.WheelDigest != "" {
				_ = w.fetchWheel(ctx, job)
			}
			dur, logContent, err := w.Runner.Run(ctx, job)
			results[i] = result{
				job:      job,
				duration: dur,
				log:      logContent,
				err:      err,
				attempt:  reqAttempts[queueKey(job.Name, job.Version)],
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
		}
		if res.job.WheelDigest != "" {
			meta["wheel_digest"] = res.job.WheelDigest
			if u := w.casURL(artifact.ID{Type: artifact.WheelType, Digest: res.job.WheelDigest}); u != "" {
				meta["wheel_url"] = u
			} else if res.job.WheelDigest != "" {
				if u := w.objectURL(res.job); u != "" {
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

		wheelURL := ""
		if v, ok := meta["wheel_url"].(string); ok {
			wheelURL = v
		}
		entry := map[string]any{
			"name":         res.job.Name,
			"version":      res.job.Version,
			"status":       status,
			"python_tag":   res.job.PythonTag,
			"platform_tag": res.job.PlatformTag,
			"wheel":        wheelURL,
			"metadata":     meta,
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
			if u := w.casURL(artifact.ID{Type: artifact.WheelType, Digest: res.job.WheelDigest}); u != "" {
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

		if res.err != nil && w.shouldRequeue(reqAttempts, res.job) {
			_ = w.Queue.Enqueue(ctx, queue.Request{
				Package:       res.job.Name,
				Version:       res.job.Version,
				PythonVersion: res.job.PythonVersion,
				PythonTag:     res.job.PythonTag,
				PlatformTag:   res.job.PlatformTag,
				Recipes:       res.job.Recipes,
				Attempts:      res.attempt + 1,
				EnqueuedAt:    time.Now().Unix(),
			})
		} else if res.err == nil {
			w.uploadArtifacts(ctx, res.job)
		}
	}

	if len(manifestEntries) > 0 {
		writeManifest(w.Cfg.OutputDir, manifestEntries)
	}
	return firstErr
}

func (w *Worker) match(ctx context.Context, reqs []queue.Request) []runner.Job {
	w.mu.Lock()
	snap := w.planSnap
	w.mu.Unlock()

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
			jobs = append(jobs, runner.Job{
				Name:          node.Name,
				Version:       node.Version,
				PythonVersion: firstNonEmpty(req.PythonVersion, node.PythonVersion),
				PythonTag:     firstNonEmpty(node.PythonTag, pyTagFromVersion(firstNonEmpty(req.PythonVersion, node.PythonVersion))),
				PlatformTag:   node.PlatformTag,
				Recipes:       req.Recipes,
				WheelDigest:   wheelDigest,
				WheelAction:   wheelAction,
				PackPaths:     w.resolvePacks(ctx, packIDs),
				RuntimePath:   w.fetchRuntime(ctx, firstNonEmpty(req.PythonVersion, node.PythonVersion), runtimeID),
				RuntimeDigest: runtimeID.Digest,
				PackDigests:   packDigests(packIDs),
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

func (w *Worker) objectURL(job runner.Job) string {
	if w.Store == nil {
		return ""
	}
	key := fmt.Sprintf("%s/%s/", strings.ToLower(job.Name), job.Version)
	if os, ok := w.Store.(interface{ URL(string) string }); ok {
		return os.URL(key)
	}
	return ""
}

// requestsFromPlan seeds work items directly from the current plan when the queue is empty.
func (w *Worker) requestsFromPlan() []queue.Request {
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

// BuildWorker constructs a worker from config.
func BuildWorker(cfg Config) (*Worker, error) {
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
		key := fmt.Sprintf("%s/%s/%s", strings.ToLower(job.Name), job.Version, e.Name())
		_ = store.Put(ctx, key, data, "application/octet-stream")
	}
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
	return w.Fetcher.Fetch(ctx, artifact.ID{Type: artifact.WheelType, Digest: job.WheelDigest}, destPath)
}

func (w *Worker) resolvePacks(ctx context.Context, ids []artifact.ID) []string {
	if len(ids) == 0 || w.Fetcher.BaseURL == "" {
		return nil
	}
	var paths []string
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
		if err := w.Fetcher.Fetch(ctx, id, destPath); err != nil {
			continue
		}
		w.packPath[id.Digest] = destPath
		paths = append(paths, destPath)
	}
	return paths
}

func (w *Worker) fetchRuntime(ctx context.Context, pythonVersion string, rtID artifact.ID) string {
	if w.Fetcher.BaseURL == "" || pythonVersion == "" || rtID.Digest == "" {
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
	if err := w.Fetcher.Fetch(ctx, rtID, destPath); err != nil {
		return ""
	}
	return destPath
}
