package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
