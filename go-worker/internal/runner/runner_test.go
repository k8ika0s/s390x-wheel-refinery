package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFakeRunner(t *testing.T) {
	r := &FakeRunner{Dur: 50 * time.Millisecond, Log: "ok"}
	dur, logContent, err := r.Run(context.Background(), Job{Name: "pkg", Version: "1.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logContent != "ok" {
		t.Fatalf("unexpected log: %s", logContent)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
	if dur != 50*time.Millisecond {
		t.Fatalf("unexpected duration: %v", dur)
	}
}

func TestPodmanRunnerBuildArgs(t *testing.T) {
	r := &PodmanRunner{
		InputDir:    "/in",
		OutputDir:   "/out",
		CacheDir:    "/cache",
		PlatformTag: "manylinux2014_s390x",
	}
	job := Job{Name: "pkg", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Recipes: []string{"a", "b"}}
	args := r.buildArgs(job)
	joined := strings.Join(args, " ")
	want := []string{
		"-v /in:/input:ro",
		"-v /out:/output",
		"-v /cache:/cache",
		"-e JOB_NAME=pkg",
		"-e JOB_VERSION=1.0.0",
		"-e PYTHON_TAG=cp311",
		"-e PLATFORM_TAG=manylinux2014_s390x",
		"-e RECIPES=a,b",
		"refinery-rocky:latest",
	}
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Fatalf("missing arg %q in %q", w, joined)
		}
	}
}

func TestPodmanRunnerBuildArgsUsesPackPrefixPaths(t *testing.T) {
	r := &PodmanRunner{
		OutputDir: "/out",
		CacheDir:  "/cache",
	}
	job := Job{
		Name:      "pkg",
		Version:   "1.0.0",
		PackPaths: []string{"/packs/openssl"},
	}
	args := strings.Join(r.buildArgs(job), " ")
	if !strings.Contains(args, "-v /packs/openssl:/opt/packs/pack0:ro") {
		t.Fatalf("expected pack mount in args: %s", args)
	}
	if !strings.Contains(args, "-e DEPS_PREFIXES=/opt/packs/pack0") {
		t.Fatalf("expected pack prefix env in args: %s", args)
	}
	if strings.Contains(args, "/opt/packs/pack0/usr/local") {
		t.Fatalf("unexpected nested usr/local prefix in args: %s", args)
	}
}

func TestPodmanRunnerBuildCmdPrefersRuntimePythonCandidates(t *testing.T) {
	r := &PodmanRunner{}
	cmd := strings.Join(r.buildCmd(Job{}), "\n")
	want := []string{
		`if [ -z "${PYTHON_BIN:-}" ] && [ -z "${PYTHON_PATH:-}" ] && [ -n "${RUNTIME_PATH:-}" ]; then`,
		`"${RUNTIME_PATH}/bin/python3"`,
		`"${RUNTIME_PATH}"/bin/python3.*`,
		`"${RUNTIME_PATH}/bin/python"`,
	}
	for _, fragment := range want {
		if !strings.Contains(cmd, fragment) {
			t.Fatalf("expected runtime python selection fragment %q in command: %s", fragment, cmd)
		}
	}
}

// PodmanRunner now fails if podman is missing; ensure error is returned.
func TestPodmanRunnerNoBinary(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer func() { _ = os.Setenv("PATH", origPath) }()
	_ = os.Setenv("PATH", "")

	r := &PodmanRunner{}
	_, _, err := r.Run(context.Background(), Job{Name: "pkg", Version: "1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "podman binary not found") {
		t.Fatalf("expected podman missing error, got %v", err)
	}
}

func TestPodmanRunnerFailure(t *testing.T) {
	// Use /bin/false (or "false") to force non-zero exit and ensure we propagate errors and output.
	bin := "false"
	r := &PodmanRunner{Bin: bin, InputDir: "/in", OutputDir: "/out", CacheDir: "/cache"}
	_, logContent, err := r.Run(context.Background(), Job{Name: "pkg", Version: "1.0.0"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if logContent == "" {
		t.Logf("no output returned (expected with %s)", bin)
	}
}
