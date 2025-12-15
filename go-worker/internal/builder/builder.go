package builder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// PackBuildOpts describes inputs for building a pack artifact.
type PackBuildOpts struct {
	Digest string
	Meta   map[string]any
}

// RuntimeBuildOpts describes inputs for building a runtime artifact.
type RuntimeBuildOpts struct {
	Digest        string
	PythonVersion string
	Policy        string
	Meta          map[string]any
}

// BuildPack writes a simple tar artifact with a manifest describing the pack.
func BuildPack(path string, opts PackBuildOpts) error {
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
