package builder

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
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
	// Shell command to run before tar creation; receives PACK_OUTPUT dir in env.
	Cmd string
}

// BuildPack executes a pack recipe into a real output tree and archives that tree.
func BuildPack(path string, opts PackBuildOpts) error {
	if strings.TrimSpace(opts.Cmd) == "" {
		return fmt.Errorf("pack builder command not configured")
	}
	return buildArtifact(path, opts.Cmd, []string{
		"PACK_DIGEST=" + opts.Digest,
	}, validatePackOutput)
}

// BuildRuntime executes a runtime recipe into a real output tree and archives that tree.
func BuildRuntime(path string, opts RuntimeBuildOpts) error {
	if strings.TrimSpace(opts.Cmd) == "" {
		return fmt.Errorf("runtime builder command not configured")
	}
	return buildArtifact(path, opts.Cmd, []string{
		"PACK_DIGEST=" + opts.Digest,
		"PYTHON_VERSION=" + opts.PythonVersion,
		"RUNTIME_POLICY=" + opts.Policy,
	}, validateRuntimeOutput)
}

func buildArtifact(path, cmd string, extraEnv []string, validate func(string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	outputDir, err := os.MkdirTemp(filepath.Dir(path), "refinery-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(outputDir)
	if err := runCommand(cmd, outputDir, extraEnv); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(outputDir); err != nil {
			return err
		}
	}
	return writeTar(path, outputDir)
}

func writeTar(path, sourceDir string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	if err := archiveTree(tw, sourceDir); err != nil {
		_ = tw.Close()
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

func archiveTree(tw *tar.Writer, sourceDir string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == sourceDir {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.ModTime = time.Unix(0, 0)
		if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

func validatePackOutput(outputDir string) error {
	if err := validateCommonOutput(outputDir); err != nil {
		return err
	}
	prefixDir := filepath.Join(outputDir, "usr", "local")
	if fi, err := os.Stat(prefixDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("pack output missing prefix dir %s", prefixDir)
	}
	return nil
}

func validateRuntimeOutput(outputDir string) error {
	if err := validateCommonOutput(outputDir); err != nil {
		return err
	}
	prefixDir := filepath.Join(outputDir, "usr", "local")
	if fi, err := os.Stat(prefixDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("runtime output missing prefix dir %s", prefixDir)
	}
	for _, candidate := range []string{
		filepath.Join(prefixDir, "bin", "python3"),
		filepath.Join(prefixDir, "bin", "python"),
	} {
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return nil
		}
	}
	return fmt.Errorf("runtime output missing python interpreter")
}

func validateCommonOutput(outputDir string) error {
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if fi, err := os.Stat(manifestPath); err != nil || fi.IsDir() {
		return fmt.Errorf("artifact output missing manifest.json")
	}
	hasPayload := false
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "manifest.json" {
			continue
		}
		hasPayload = true
		break
	}
	if !hasPayload {
		return fmt.Errorf("artifact output missing payload beyond manifest.json")
	}
	return nil
}

// runCommand executes a shell command, setting PACK_OUTPUT and extra env vars.
func runCommand(cmd, outputDir string, extraEnv []string) error {
	c := exec.Command("sh", "-c", cmd)
	env := filterEnv(os.Environ(), "PACK_OUTPUT")
	for _, entry := range extraEnv {
		if entry == "" {
			continue
		}
		key, _, ok := strings.Cut(entry, "=")
		if ok && key != "" {
			env = filterEnv(env, key)
		}
	}
	c.Env = append(env, "PACK_OUTPUT="+outputDir)
	c.Env = append(c.Env, extraEnv...)
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
