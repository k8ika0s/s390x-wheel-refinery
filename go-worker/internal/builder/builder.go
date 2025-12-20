package builder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		if err := runCommand(opts.Cmd, "PACK_OUTPUT", filepath.Dir(path)); err != nil {
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
		if err := runCommand(opts.Cmd, "RUNTIME_OUTPUT", filepath.Dir(path)); err != nil {
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
	// add a minimal payload to avoid manifest-only artifacts
	keepData := []byte("keep")
	if err := tw.WriteHeader(&tar.Header{
		Name:    "usr/local/.keep",
		Mode:    0o644,
		Size:    int64(len(keepData)),
		ModTime: time.Unix(0, 0),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(keepData); err != nil {
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
func runCommand(cmd, outputEnv, destDir string) error {
	if cmd == "" {
		return nil
	}
	tmp, err := os.MkdirTemp(destDir, "refinery-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	cmd = fmt.Sprintf("%s=%s %s", outputEnv, shellQuote(tmp), cmd)
	c := exec.Command("sh", "-c", cmd)
	env := filterEnv(os.Environ(), outputEnv)
	c.Env = append(env, outputEnv+"="+tmp)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func filterEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
