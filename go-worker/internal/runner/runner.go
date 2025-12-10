package runner

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Job describes a build job the worker executes.
type Job struct {
	Name        string
	Version     string
	PythonTag   string
	PlatformTag string
	Recipes     []string
}

// Runner executes build jobs.
type Runner interface {
	Run(ctx context.Context, job Job) (duration time.Duration, logContent string, err error)
}

// PodmanRunner runs jobs in a podman container.
type PodmanRunner struct {
	Image       string
	InputDir    string
	OutputDir   string
	CacheDir    string
	PythonTag   string
	PlatformTag string
	Bin         string
}

// Run executes a placeholder podman command. In a real implementation this would
// invoke the build script inside the container. Here we simulate success for tests.
func (p *PodmanRunner) Run(ctx context.Context, job Job) (time.Duration, string, error) {
	start := time.Now()
	if p.Bin == "" {
		// Stubbed podman: simulate success.
		return time.Since(start), "podman stub", nil
	}
	cmd := exec.CommandContext(ctx, p.Bin, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return time.Since(start), string(output), fmt.Errorf("podman run failed: %w", err)
	}
	return time.Since(start), string(output), nil
}

// FakeRunner is used in tests.
type FakeRunner struct {
	Calls []Job
	Err   error
	Dur   time.Duration
	Log   string
}

func (f *FakeRunner) Run(ctx context.Context, job Job) (time.Duration, string, error) {
	f.Calls = append(f.Calls, job)
	return f.Dur, f.Log, f.Err
}
