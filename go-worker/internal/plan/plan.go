package plan

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Node represents a plan entry.
type Node struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PythonTag   string `json:"python_tag"`
	PlatformTag string `json:"platform_tag"`
	Action      string `json:"action"`
}

// Snapshot is the structure stored in plan.json.
type Snapshot struct {
	RunID string `json:"run_id"`
	Plan  []Node `json:"plan"`
}

// Write writes a snapshot to the given path.
func Write(path string, snap Snapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), data, 0o644)
}

// Load loads a plan snapshot from disk.
func Load(path string) (Snapshot, error) {
	var snap Snapshot
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return snap, err
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return snap, fmt.Errorf("unmarshal plan: %w", err)
	}
	return snap, nil
}

// GenerateViaPython shells out to the Python CLI to build a plan when plan.json is missing.
func GenerateViaPython(inputDir, cacheDir, pythonVersion, platformTag string) (Snapshot, error) {
	cmd := exec.Command("refinery", "--input", inputDir, "--output", cacheDir, "--cache", cacheDir, "--python", pythonVersion, "--platform-tag", platformTag, "--skip-known-failures", "--no-system-recipes")
	cmd.Env = append(os.Environ(), "REFINERY_PLAN_ONLY=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Snapshot{}, fmt.Errorf("python plan failed: %w output=%s", err, string(out))
	}
	return Load(filepath.Join(cacheDir, "plan.json"))
}

// Generate builds a plan using the Go resolver and writes it to cacheDir/plan.json.
func Generate(inputDir, cacheDir, pythonVersion, platformTag string) (Snapshot, error) {
	snap, err := compute(inputDir, pythonVersion, platformTag)
	if err != nil {
		return Snapshot{}, err
	}
	path := filepath.Join(cacheDir, "plan.json")
	if err := Write(path, snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// compute walks input wheels and decides reuse vs build for the target tags.
func compute(inputDir, pythonVersion, platformTag string) (Snapshot, error) {
	files, err := os.ReadDir(inputDir)
	if err != nil {
		return Snapshot{}, err
	}
	pyTag := normalizePyTag(pythonVersion)
	var nodes []Node

	seen := make(map[string]bool)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".whl") {
			continue
		}
		info, err := parseWheelFilename(f.Name())
		if err != nil {
			continue
		}
		key := info.Name + "::" + info.Version
		if seen[key] {
			continue
		}
		seen[key] = true

		if isCompatible(info, pyTag, platformTag) {
			nodes = append(nodes, Node{
				Name:        info.Name,
				Version:     info.Version,
				PythonTag:   pyTag,
				PlatformTag: platformTag,
				Action:      "reuse",
			})
		} else {
			nodes = append(nodes, Node{
				Name:        info.Name,
				Version:     info.Version,
				PythonTag:   pyTag,
				PlatformTag: platformTag,
				Action:      "build",
			})
		}
	}
	return Snapshot{RunID: newRunID(), Plan: nodes}, nil
}

type wheelInfo struct {
	Name        string
	Version     string
	PythonTag   string
	AbiTag      string
	PlatformTag string
}

func parseWheelFilename(name string) (wheelInfo, error) {
	base := strings.TrimSuffix(name, ".whl")
	parts := strings.Split(base, "-")
	if len(parts) < 5 {
		return wheelInfo{}, fmt.Errorf("invalid wheel filename: %s", name)
	}
	platform := parts[len(parts)-1]
	abi := parts[len(parts)-2]
	py := parts[len(parts)-3]
	version := parts[len(parts)-4]
	pkgParts := parts[:len(parts)-4]
	pkg := strings.ReplaceAll(strings.Join(pkgParts, "-"), "_", "-")
	return wheelInfo{
		Name:        pkg,
		Version:     version,
		PythonTag:   py,
		AbiTag:      abi,
		PlatformTag: platform,
	}, nil
}

func isCompatible(w wheelInfo, targetPy, targetPlatform string) bool {
	pyOK := w.PythonTag == targetPy || strings.HasPrefix(w.PythonTag, "py3") || strings.HasPrefix(w.PythonTag, "cp3")
	platOK := w.PlatformTag == "any" || w.PlatformTag == targetPlatform
	abiOK := w.AbiTag == "none" || strings.HasPrefix(w.AbiTag, "cp3")
	return pyOK && platOK && abiOK
}

func normalizePyTag(pythonVersion string) string {
	if strings.HasPrefix(pythonVersion, "cp") {
		return pythonVersion
	}
	trimmed := strings.ReplaceAll(pythonVersion, ".", "")
	if !strings.HasPrefix(trimmed, "3") {
		return "cp" + trimmed
	}
	return "cp" + trimmed
}

func newRunID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 12)
	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
