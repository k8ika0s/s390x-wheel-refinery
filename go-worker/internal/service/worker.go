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
			snap, err = plan.Generate(w.Cfg.InputDir, w.Cfg.CacheDir, w.Cfg.PythonVersion, w.Cfg.PlatformTag)
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
		return nil
	}

	reqAttempts := make(map[string]int)
	for _, r := range reqs {
		key := queueKey(r.Package, r.Version)
		if r.Attempts > reqAttempts[key] {
			reqAttempts[key] = r.Attempts
		}
	}

	jobs := w.match(reqs)
	results := make([]result, len(jobs))
	g, ctx := errgroup.WithContext(ctx)
	for i, job := range jobs {
		i, job := i, job
		g.Go(func() error {
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

		entry := map[string]any{
			"name":         res.job.Name,
			"version":      res.job.Version,
			"status":       status,
			"python_tag":   res.job.PythonTag,
			"platform_tag": res.job.PlatformTag,
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
				Package:     res.job.Name,
				Version:     res.job.Version,
				PythonTag:   res.job.PythonTag,
				PlatformTag: res.job.PlatformTag,
				Recipes:     res.job.Recipes,
				Attempts:    res.attempt + 1,
				EnqueuedAt:  time.Now().Unix(),
			})
		}
	}

	if len(manifestEntries) > 0 {
		writeManifest(w.Cfg.OutputDir, manifestEntries)
	}
	return firstErr
}

func (w *Worker) match(reqs []queue.Request) []runner.Job {
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
			jobs = append(jobs, runner.Job{
				Name:        node.Name,
				Version:     node.Version,
				PythonTag:   node.PythonTag,
				PlatformTag: node.PlatformTag,
				Recipes:     req.Recipes,
			})
		}
	}
	return jobs
}

func equalsIgnoreCase(a, b string) bool {
	return strings.EqualFold(a, b)
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
	return &Worker{Queue: q, Runner: r, Reporter: rep, Cfg: cfg}, nil
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
