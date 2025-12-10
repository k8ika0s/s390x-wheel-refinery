package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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
	Timeout     time.Duration
	RunCmd      []string
}

// Run executes a placeholder podman command. In a real implementation this would
// invoke the build script inside the container. Here we simulate success for tests.
func (p *PodmanRunner) Run(ctx context.Context, job Job) (time.Duration, string, error) {
	start := time.Now()
	bin := p.Bin
	if bin == "" {
		// Stubbed podman: simulate success.
		return time.Since(start), "podman stub", nil
	}
	args := p.buildArgs(job)

	runCtx := ctx
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}
	execCmd := exec.CommandContext(runCtx, bin, args...)
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return time.Since(start), string(output), fmt.Errorf("podman run failed: %w", err)
	}
	return time.Since(start), string(output), nil
}

func (p *PodmanRunner) defaultImage() string {
	if p.Image != "" {
		return p.Image
	}
	switch p.PlatformTag {
	case "manylinux2014_s390x":
		return "refinery-rocky:latest"
	default:
		return "refinery-rocky:latest"
	}
}

func (p *PodmanRunner) buildCmd(job Job) []string {
	if len(p.RunCmd) > 0 {
		return p.RunCmd
	}
	// Default to a shell command that echoes the job; replace with real build entrypoint as needed.
	return []string{"/bin/sh", "-c", fmt.Sprintf("echo build %s==%s", job.Name, job.Version)}
}

// buildArgs assembles the podman arguments with mounts, env, image, and command.
func (p *PodmanRunner) buildArgs(job Job) []string {
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/input:ro", p.InputDir),
		"-v", fmt.Sprintf("%s:/output", p.OutputDir),
		"-v", fmt.Sprintf("%s:/cache", p.CacheDir),
		"-e", fmt.Sprintf("JOB_NAME=%s", job.Name),
		"-e", fmt.Sprintf("JOB_VERSION=%s", job.Version),
		"-e", fmt.Sprintf("PYTHON_TAG=%s", job.PythonTag),
		"-e", fmt.Sprintf("PLATFORM_TAG=%s", job.PlatformTag),
	}
	if len(job.Recipes) > 0 {
		args = append(args, "-e", fmt.Sprintf("RECIPES=%s", strings.Join(job.Recipes, ",")))
	}
	image := p.defaultImage()
	cmdArgs := p.buildCmd(job)
	args = append(args, image)
	args = append(args, cmdArgs...)
	return args
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
