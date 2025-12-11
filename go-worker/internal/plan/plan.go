package plan

import (
	"archive/zip"
	"bytes"
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

// Options control resolver behavior.
type Options struct {
	IndexURL         string
	ExtraIndexURL    string
	UpgradeStrategy  string // pinned (default) or eager
	MaxDeps          int    // safety cap for dependency expansion
	PackageOverrides map[string]string
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
func Generate(inputDir, cacheDir, pythonVersion, platformTag string, indexURL, extraIndexURL, strategy string) (Snapshot, error) {
	opts := Options{
		IndexURL:         indexURL,
		ExtraIndexURL:    extraIndexURL,
		UpgradeStrategy:  strategy,
		MaxDeps:          1000,
		PackageOverrides: loadOverridesFromEnv(),
	}
	snap, err := computeWithResolver(inputDir, pythonVersion, platformTag, opts, &IndexClient{BaseURL: indexURL, ExtraIndexURL: extraIndexURL})
	if err != nil {
		return Snapshot{}, err
	}
	path := filepath.Join(cacheDir, "plan.json")
	if err := Write(path, snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// computeWithResolver walks input wheels and decides reuse vs build for the target tags.
func computeWithResolver(inputDir, pythonVersion, platformTag string, opts Options, resolver versionResolver) (Snapshot, error) {
	if opts.MaxDeps <= 0 {
		opts.MaxDeps = 1000
	}
	files, err := os.ReadDir(inputDir)
	if err != nil {
		return Snapshot{}, err
	}
	pyTag := normalizePyTag(pythonVersion)
	var nodes []Node
	depSeen := make(map[string]depSpec)
	var wheelCount int
	var depTruncated bool

	seen := make(map[string]bool)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".whl") {
			continue
		}
		info, err := parseWheelFilename(f.Name())
		if err != nil {
			continue
		}
		wheelCount++
		requires, _ := readRequiresDist(filepath.Join(inputDir, f.Name()))
		for _, dep := range requires {
			if dep.Name == "" {
				continue
			}
			if len(depSeen) >= opts.MaxDeps {
				depTruncated = true
				break
			}
			if existing, ok := depSeen[dep.Name]; ok && existing.Version != "" {
				continue
			}
			// resolve per strategy
			if resolver != nil && dep.Version == "" {
				if ver, err := resolver.ResolveLatest(dep.Name); err == nil {
					dep.Version = ver
				}
			}
			depSeen[dep.Name] = dep
		}

		if overrideVer, ok := opts.PackageOverrides[normalizeName(info.Name)]; ok && overrideVer != "" {
			ver := strings.TrimPrefix(strings.TrimSpace(overrideVer), "==")
			key := info.Name + "::" + ver
			if !seen[key] {
				seen[key] = true
				nodes = append(nodes, Node{
					Name:        info.Name,
					Version:     ver,
					PythonTag:   pyTag,
					PlatformTag: platformTag,
					Action:      "build",
				})
			}
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
	for dep, spec := range depSeen {
		if dep == "" {
			continue
		}
		key := dep + "::"
		if seen[key] {
			continue
		}
		version := spec.Version
		if ov, ok := opts.PackageOverrides[normalizeName(dep)]; ok && ov != "" {
			version = strings.TrimPrefix(strings.TrimSpace(ov), "==")
		}
		if version == "" {
			if resolver != nil {
				if ver, err := resolver.ResolveLatest(dep); err == nil {
					version = ver
				}
			}
			if version == "" {
				version = "latest"
			}
		}
		if opts.UpgradeStrategy == "eager" && resolver != nil {
			if ver, err := resolver.ResolveLatest(dep); err == nil && ver != "" {
				version = ver
			}
		}
		nodes = append(nodes, Node{
			Name:        dep,
			Version:     version,
			PythonTag:   pyTag,
			PlatformTag: platformTag,
			Action:      "build",
		})
	}
	if wheelCount == 0 {
		return Snapshot{}, fmt.Errorf("no wheels found in input directory %s", inputDir)
	}
	if depTruncated {
		return Snapshot{}, fmt.Errorf("dependency expansion exceeded MaxDeps (%d); increase MAX_DEPS or trim input", opts.MaxDeps)
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

// readRequiresDist extracts Requires-Dist entries from METADATA inside a wheel.
func readRequiresDist(wheelPath string) ([]depSpec, error) {
	zr, err := zip.OpenReader(wheelPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var meta []byte
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "METADATA") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			meta = buf.Bytes()
			break
		}
	}
	if len(meta) == 0 {
		return nil, nil
	}
	return parseRequiresDist(string(meta)), nil
}

type depSpec struct {
	Name    string
	Version string
}

type versionResolver interface {
	ResolveLatest(name string) (string, error)
}

func normalizeName(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
}

func loadOverridesFromEnv() map[string]string {
	raw := os.Getenv("PLAN_OVERRIDES_JSON")
	if raw == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		n := normalizeName(k)
		if n != "" {
			out[n] = strings.TrimSpace(v)
		}
	}
	return out
}

func parseRequiresDist(meta string) []depSpec {
	lines := strings.Split(meta, "\n")
	var out []depSpec
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "requires-dist:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			val := strings.TrimSpace(parts[1])
			// Drop environment markers after ';'
			if semi := strings.Index(val, ";"); semi != -1 {
				val = strings.TrimSpace(val[:semi])
			}
			raw := val
			name := val
			version := ""
			// Pull out spec in parentheses, e.g., (==1.2.3) or (>=1.0)
			if idx := strings.Index(raw, "("); idx != -1 {
				name = strings.TrimSpace(raw[:idx])
				spec := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw[idx:]), "("), ")")
				spec = strings.TrimSpace(spec)
				if strings.HasPrefix(spec, "==") || strings.HasPrefix(spec, ">=") || strings.HasPrefix(spec, "~=") {
					version = spec
				}
			}
			// Handle simple whitespace-separated version like "pkg ==1.2.3"
			if version == "" {
				fields := strings.Fields(raw)
				if len(fields) >= 2 {
					name = fields[0]
					spec := strings.TrimSpace(fields[1])
					if strings.HasPrefix(spec, "==") || strings.HasPrefix(spec, ">=") || strings.HasPrefix(spec, "~=") {
						version = spec
					}
				}
			}
			// Handle inline operator without whitespace, e.g., otherpkg~=1.4
			if version == "" {
				for _, op := range []string{"==", ">=", "~="} {
					if idx := strings.Index(raw, op); idx > 0 {
						name = strings.TrimSpace(raw[:idx])
						version = raw[idx:]
						break
					}
				}
			}
			// Strip extras [extra] from the name after spec parsing
			if br := strings.Index(name, "["); br != -1 {
				name = strings.TrimSpace(name[:br])
			}
			name = strings.TrimSpace(name)
			name = strings.ToLower(strings.ReplaceAll(name, "_", "-"))
			if name != "" {
				out = append(out, depSpec{Name: name, Version: version})
			}
		}
	}
	return out
}
