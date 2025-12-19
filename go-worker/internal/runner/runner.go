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
	Name              string
	Version           string
	PythonVersion     string
	PythonTag         string
	PlatformTag       string
	Recipes           []string
	WheelDigest       string
	WheelAction       string
	RuntimePath       string
	PackPaths         []string
	RuntimeDigest     string
	PackDigests       []string
	WheelSourceDigest string
	RepairToolVersion string
	RepairPolicyHash  string
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
			return time.Since(start), "", fmt.Errorf("podman binary not found; set PODMAN_BIN")
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
	// Default command builds a wheel via pip inside the builder image using JOB_NAME/JOB_VERSION.
	return []string{
		"/bin/sh",
		"-c",
		`set -euo pipefail
spec="${JOB_NAME:-}"
if [ -z "$spec" ]; then
  echo "runner: JOB_NAME is required" >&2
  exit 1
fi
if [ -n "${JOB_VERSION:-}" ]; then
  spec="${spec}==${JOB_VERSION}"
fi
PYBIN="${PYTHON_BIN:-${PYTHON_PATH:-python3}}"
export PIP_NO_INPUT=1
export PIP_CACHE_DIR="${PIP_CACHE_DIR:-/cache/pip}"
if [ -n "${DEPS_PREFIXES:-}" ]; then
  pc_paths=""
  for pfx in $(echo "${DEPS_PREFIXES}" | tr ':' ' '); do
    pc_paths="${pc_paths}${pfx}/lib/pkgconfig:"
    export CFLAGS="${CFLAGS:-} -I${pfx}/include"
    export LDFLAGS="${LDFLAGS:-} -L${pfx}/lib"
    export LD_LIBRARY_PATH="${LD_LIBRARY_PATH:-}:${pfx}/lib:${pfx}/lib64"
  done
  export PKG_CONFIG_PATH="${pc_paths}${PKG_CONFIG_PATH:-}"
fi
if [ -n "${RECIPES:-}" ]; then
  IFS=',' read -r -a recipe_list <<< "${RECIPES}"
  apt_pkgs=()
  dnf_pkgs=()
  pip_pkgs=()
  for r in "${recipe_list[@]}"; do
    r="$(echo "$r" | xargs)"
    if [ -z "$r" ]; then
      continue
    fi
    case "$r" in
      apt:*) apt_pkgs+=("${r#apt:}") ;;
      dnf:*) dnf_pkgs+=("${r#dnf:}") ;;
      pip:*) pip_pkgs+=("${r#pip:}") ;;
      env:*) export "${r#env:}" ;;
      *) ;;
    esac
  done
  if command -v dnf >/dev/null 2>&1 && [ ${#dnf_pkgs[@]} -gt 0 ]; then
    dnf -y install "${dnf_pkgs[@]}"
  fi
  if command -v apt-get >/dev/null 2>&1 && [ ${#apt_pkgs[@]} -gt 0 ]; then
    apt-get update
    apt-get install -y "${apt_pkgs[@]}"
  fi
  if [ ${#pip_pkgs[@]} -gt 0 ]; then
    "${PYBIN}" -m pip install "${pip_pkgs[@]}"
  fi
fi
exec "${PYBIN}" -m pip wheel "${spec}" -w /output --no-deps`,
	}
}

// buildArgs assembles the podman arguments with mounts, env, image, and command.
func (p *PodmanRunner) buildArgs(job Job) []string {
	tag := job.PythonTag
	if tag == "" {
		tag = pyTagFromVersion(job.PythonVersion)
	}
	var depPrefixes []string
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/output", p.OutputDir),
		"-v", fmt.Sprintf("%s:/cache", p.CacheDir),
		"-e", fmt.Sprintf("JOB_NAME=%s", job.Name),
		"-e", fmt.Sprintf("JOB_VERSION=%s", job.Version),
	}
	if p.InputDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/input:ro", p.InputDir))
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
	if job.RuntimePath != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/opt/runtime:ro", job.RuntimePath))
		args = append(args, "-e", "RUNTIME_PATH=/opt/runtime")
		args = append(args, "-e", "PYTHONHOME=/opt/runtime")
		args = append(args, "-e", "PATH=/opt/runtime/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin")
		args = append(args, "-e", "LD_LIBRARY_PATH=/opt/runtime/lib:/opt/runtime/lib64:/opt/packs/lib")
	}
	for i, pth := range job.PackPaths {
		args = append(args, "-v", fmt.Sprintf("%s:/opt/packs/pack%d:ro", pth, i))
		depPrefixes = append(depPrefixes, fmt.Sprintf("/opt/packs/pack%d/usr/local", i))
	}
	if job.RuntimeDigest != "" {
		args = append(args, "-e", fmt.Sprintf("RUNTIME_DIGEST=%s", job.RuntimeDigest))
	}
	if len(job.PackDigests) > 0 {
		args = append(args, "-e", fmt.Sprintf("PACK_DIGESTS=%s", strings.Join(job.PackDigests, ",")))
	}
	if len(depPrefixes) > 0 {
		args = append(args, "-e", fmt.Sprintf("DEPS_PREFIXES=%s", strings.Join(depPrefixes, ":")))
	}
	if job.WheelSourceDigest != "" {
		args = append(args, "-e", fmt.Sprintf("WHEEL_SOURCE_DIGEST=%s", job.WheelSourceDigest))
	}
	if job.RepairToolVersion != "" {
		args = append(args, "-e", fmt.Sprintf("REPAIR_TOOL_VERSION=%s", job.RepairToolVersion))
	}
	if job.RepairPolicyHash != "" {
		args = append(args, "-e", fmt.Sprintf("REPAIR_POLICY_HASH=%s", job.RepairPolicyHash))
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
