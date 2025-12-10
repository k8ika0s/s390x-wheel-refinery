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

// LoadPlan reads plan.json if present.
func (w *Worker) LoadPlan() error {
	path := filepath.Join(w.Cfg.OutputDir, "plan.json")
	snap, err := plan.Load(path)
	if err != nil {
		// fall back to cache plan
		path = filepath.Join(w.Cfg.CacheDir, "plan.json")
		snap, err = plan.Load(path)
		if err != nil {
			// as a last resort, try Python CLI to generate plan
			snap, err = plan.GenerateViaPython(w.Cfg.InputDir, w.Cfg.CacheDir, w.Cfg.PythonVersion, w.Cfg.PlatformTag)
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

	jobs := w.match(reqs)
	g, ctx := errgroup.WithContext(ctx)
	for _, job := range jobs {
		job := job
		g.Go(func() error {
			dur, logContent, err := w.Runner.Run(ctx, job)
			if w.Reporter != nil {
				_ = w.Reporter.PostLog(map[string]any{"name": job.Name, "version": job.Version, "content": logContent})
			}
			if err != nil {
				return err
			}
			_ = w.Reporter.PostManifest([]map[string]any{{
				"name": job.Name, "version": job.Version, "status": "built", "python_tag": job.PythonTag, "platform_tag": job.PlatformTag,
				"metadata": map[string]any{"duration_ms": dur.Milliseconds()},
			}})
			_ = w.Reporter.PostEvent(map[string]any{
				"name":         job.Name,
				"version":      job.Version,
				"python_tag":   job.PythonTag,
				"platform_tag": job.PlatformTag,
				"status":       "built",
				"timestamp":    time.Now().Unix(),
				"metadata":     map[string]any{"duration_ms": dur.Milliseconds()},
			})
			return nil
		})
	}
	return g.Wait()
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
	r := &runner.PodmanRunner{Image: cfg.ContainerImage, InputDir: cfg.InputDir, OutputDir: cfg.OutputDir, CacheDir: cfg.CacheDir, PythonTag: cfg.PythonVersion, PlatformTag: cfg.PlatformTag, Bin: cfg.PodmanBin}
	rep := &reporter.Client{BaseURL: strings.TrimRight(cfg.ControlPlaneURL, "/"), Token: cfg.ControlPlaneToken}
	return &Worker{Queue: q, Runner: r, Reporter: rep, Cfg: cfg}, nil
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
