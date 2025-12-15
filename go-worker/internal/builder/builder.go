package builder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// PackBuildOpts describes inputs for building a pack artifact.
type PackBuildOpts struct {
	Digest string
	Meta   map[string]any
	// Shell command to run before tar creation; receives PACK_OUTPUT dir in env.
	Cmd string
}

// RuntimeBuildOpts describes inputs for building a runtime artifact.
type RuntimeBuildOpts struct {
	Digest        string
	PythonVersion string
	Policy        string
	Meta          map[string]any
	// Shell command to run before tar creation; receives RUNTIME_OUTPUT dir in env.
	Cmd string
}

// BuildPack writes a simple tar artifact with a manifest describing the pack.
func BuildPack(path string, opts PackBuildOpts) error {
	if opts.Cmd != "" {
		if err := runCommand(opts.Cmd, "PACK_OUTPUT"); err != nil {
			return err
		}
	}
	manifest := map[string]any{
		"kind":        "pack",
		"digest":      opts.Digest,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"meta":        opts.Meta,
	}
	return writeTar(path, manifest)
}

// BuildRuntime writes a simple tar artifact with a manifest describing the runtime.
func BuildRuntime(path string, opts RuntimeBuildOpts) error {
	if opts.Cmd != "" {
		if err := runCommand(opts.Cmd, "RUNTIME_OUTPUT"); err != nil {
			return err
		}
	}
	manifest := map[string]any{
		"kind":           "runtime",
		"digest":         opts.Digest,
		"python_version": opts.PythonVersion,
		"policy":         opts.Policy,
		"generatedAt":    time.Now().UTC().Format(time.RFC3339),
		"meta":           opts.Meta,
	}
	return writeTar(path, manifest)
}

func writeTar(path string, manifest map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
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
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return nil
}

// runCommand executes a shell command if provided, setting an output dir env var.
func runCommand(cmd, outputEnv string) error {
	if cmd == "" {
		return nil
	}
	tmp, err := os.MkdirTemp("", "refinery-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	c := exec.Command("sh", "-c", cmd)
	c.Env = append(os.Environ(), outputEnv+"="+tmp)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
