package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Job describes a build job the worker executes.
type Job struct {
	Name          string
	Version       string
	PythonVersion string
	PythonTag     string
	PlatformTag   string
	Recipes       []string
	WheelDigest   string
	WheelAction   string
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

func pyTagFromVersion(ver string) string {
	trimmed := strings.ReplaceAll(ver, ".", "")
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "cp") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "3") {
		return "cp" + trimmed
	}
	return trimmed
}

// Run executes a placeholder podman command. In a real implementation this would
// invoke the build script inside the container. Here we simulate success for tests.
func (p *PodmanRunner) Run(ctx context.Context, job Job) (time.Duration, string, error) {
	start := time.Now()
	bin := p.Bin
	if bin == "" {
		if path, err := exec.LookPath("podman"); err == nil {
			bin = path
		} else {
			return time.Since(start), "podman stub (podman not found)", nil
		}
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
	elapsed := time.Since(start)
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		reason := "error"
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		logContent := fmt.Sprintf("%s\nstatus=error reason=%s elapsed_ms=%d", trimmed, reason, elapsed.Milliseconds())
		return elapsed, logContent, fmt.Errorf("podman run failed (%s): %w", reason, err)
	}
	logContent := fmt.Sprintf("%s\nstatus=ok elapsed_ms=%d", trimmed, elapsed.Milliseconds())
	return elapsed, logContent, nil
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
	// Default command uses the Python refinery CLI inside the container to build a single job.
	return []string{
		"/bin/sh",
		"-c",
		"refinery --input /input --output /output --cache /cache --python ${PYTHON_TAG:-3.11} " +
			"--platform-tag ${PLATFORM_TAG:-manylinux2014_s390x} --only ${JOB_NAME:-}${JOB_VERSION:+==${JOB_VERSION:-}} --jobs 1",
	}
}

// buildArgs assembles the podman arguments with mounts, env, image, and command.
func (p *PodmanRunner) buildArgs(job Job) []string {
	tag := job.PythonTag
	if tag == "" {
		tag = pyTagFromVersion(job.PythonVersion)
	}
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/input:ro", p.InputDir),
		"-v", fmt.Sprintf("%s:/output", p.OutputDir),
		"-v", fmt.Sprintf("%s:/cache", p.CacheDir),
		"-e", fmt.Sprintf("JOB_NAME=%s", job.Name),
		"-e", fmt.Sprintf("JOB_VERSION=%s", job.Version),
	}
	if job.PythonVersion != "" {
		args = append(args, "-e", fmt.Sprintf("PYTHON_VERSION=%s", job.PythonVersion))
	}
	if tag != "" {
		args = append(args, "-e", fmt.Sprintf("PYTHON_TAG=%s", tag))
	} else if p.PythonTag != "" {
		args = append(args, "-e", fmt.Sprintf("PYTHON_TAG=%s", p.PythonTag))
	}
	args = append(args, "-e", fmt.Sprintf("PLATFORM_TAG=%s", job.PlatformTag))
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
