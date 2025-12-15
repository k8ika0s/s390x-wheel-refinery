package plan

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/cas"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FlatNode represents a legacy plan entry.
type FlatNode struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	PythonVersion string `json:"python_version,omitempty"`
	PythonTag     string `json:"python_tag"`
	PlatformTag   string `json:"platform_tag"`
	Action        string `json:"action"`
}

// Snapshot is the structure stored in plan.json.
type Snapshot struct {
	RunID string     `json:"run_id"`
	Plan  []FlatNode `json:"plan"`
	// DAG will carry richer artifact nodes when populated by the planner (optional for now).
	DAG []DAGNode `json:"dag,omitempty"`
	CAS *CASInfo  `json:"cas,omitempty"`
}

// CASInfo records the CAS registry settings used for planning (for downstream reuse).
type CASInfo struct {
	RegistryURL  string `json:"registry_url,omitempty"`
	RegistryRepo string `json:"registry_repo,omitempty"`
}

// Options control resolver behavior.
type Options struct {
	IndexURL         string
	ExtraIndexURL    string
	IndexUsername    string
	IndexPassword    string
	UpgradeStrategy  string // pinned (default) or eager
	MaxDeps          int    // safety cap for dependency expansion
	PackageOverrides map[string]string
	RequirementsPath string
	ConstraintsPath  string
	PackCatalog      *pack.Catalog
	ArtifactStore    cas.Store
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
func Generate(
	inputDir,
	cacheDir,
	pythonVersion,
	platformTag string,
	indexURL,
	extraIndexURL,
	strategy,
	requirementsPath,
	constraintsPath string,
	catalog *pack.Catalog,
	store cas.Store,
	casRegistryURL,
	casRegistryRepo string,
) (Snapshot, error) {
	maxDeps := loadMaxDepsFromEnv()
	if maxDeps <= 0 {
		maxDeps = 1000
	}
	if requirementsPath == "" {
		requirementsPath = filepath.Join(inputDir, "requirements.txt")
	}
	if constraintsPath == "" {
		constraintsPath = filepath.Join(inputDir, "constraints.txt")
	}
	opts := Options{
		IndexURL:         indexURL,
		ExtraIndexURL:    extraIndexURL,
		IndexUsername:    os.Getenv("INDEX_USERNAME"),
		IndexPassword:    os.Getenv("INDEX_PASSWORD"),
		UpgradeStrategy:  strategy,
		MaxDeps:          maxDeps,
		PackageOverrides: loadOverridesFromEnv(),
		RequirementsPath: requirementsPath,
		ConstraintsPath:  constraintsPath,
		PackCatalog:      catalog,
		ArtifactStore:    store,
	}
	snap, err := computeWithResolver(inputDir, pythonVersion, platformTag, opts, &IndexClient{
		BaseURL:       indexURL,
		ExtraIndexURL: extraIndexURL,
		Username:      opts.IndexUsername,
		Password:      opts.IndexPassword,
	})
	if err != nil {
		return Snapshot{}, err
	}
	if casRegistryURL != "" {
		snap.CAS = &CASInfo{RegistryURL: casRegistryURL, RegistryRepo: casRegistryRepo}
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
	store := opts.ArtifactStore
	if store == nil {
		store = cas.NullStore{}
	}
	ctx := context.TODO()
	reqs := loadRequirements(inputDir, opts.RequirementsPath)
	constraints := loadConstraints(opts.ConstraintsPath)
	files, err := os.ReadDir(inputDir)
	if err != nil {
		return Snapshot{}, err
	}
	pyTag := normalizePyTag(pythonVersion)
	var nodes []FlatNode
	var dagNodes []DAGNode
	addRepair := func(wheelID artifact.ID, meta map[string]any) {
		repairKey := artifact.RepairKey{
			InputWheelDigest:  wheelID.Digest,
			RepairToolVersion: "",
			PolicyRulesDigest: "",
		}
		repairID := artifact.ID{Type: artifact.RepairType, Digest: repairKey.Digest()}
		action := "build"
		if ok, _ := store.Has(ctx, repairID); ok {
			action = "reuse"
		}
		dagNodes = append(dagNodes, DAGNode{
			ID:       repairID,
			Type:     NodeRepair,
			Inputs:   []artifact.ID{wheelID},
			Metadata: meta,
			Action:   action,
		})
	}
	// Runtime node (shallow DAG for now)
	rtKey := artifact.RuntimeKey{Arch: "s390x", PolicyBaseDigest: "", PythonVersion: pythonVersion}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: rtKey.Digest()}
	rtAction := "build"
	if ok, _ := store.Has(ctx, rtID); ok {
		rtAction = "reuse"
	}
	dagNodes = append(dagNodes, DAGNode{
		ID:     rtID,
		Type:   NodeRuntime,
		Action: rtAction,
		Metadata: map[string]any{
			"python_version": pythonVersion,
			"python_tag":     pyTag,
			"platform_tag":   platformTag,
		},
	})
	packSeen := make(map[string]pack.PackDef)
	selectPacks := func(pkg string) ([]artifact.ID, []string) {
		if opts.PackCatalog == nil {
			return nil, nil
		}
		var ids []artifact.ID
		var digests []string
		for _, def := range opts.PackCatalog.Select(pkg, "") {
			key := artifact.PackKey{
				Arch:             "s390x",
				PolicyBaseDigest: "",
				Name:             def.Name,
				Version:          def.Version,
				RecipeDigest:     def.RecipeDigest,
			}
			id := artifact.ID{Type: artifact.PackType, Digest: key.Digest()}
			if _, ok := packSeen[id.Digest]; !ok {
				packSeen[id.Digest] = def
				action := "build"
				if ok, _ := store.Has(ctx, id); ok {
					action = "reuse"
				}
				var inputs []artifact.ID
				for _, depName := range packDependencies(def.Name) {
					if depDef, ok := opts.PackCatalog.Packs[depName]; ok {
						depKey := artifact.PackKey{Arch: "s390x", PolicyBaseDigest: "", Name: depDef.Name, Version: depDef.Version, RecipeDigest: depDef.RecipeDigest}
						inputs = append(inputs, artifact.ID{Type: artifact.PackType, Digest: depKey.Digest()})
					}
				}
				meta := map[string]any{
					"name": def.Name,
				}
				if def.Version != "" {
					meta["version"] = def.Version
				}
				if def.RecipeDigest != "" {
					meta["recipe_digest"] = def.RecipeDigest
				}
				if def.Note != "" {
					meta["note"] = def.Note
				}
				dagNodes = append(dagNodes, DAGNode{
					ID:       id,
					Type:     NodePack,
					Inputs:   inputs,
					Metadata: meta,
					Action:   action,
				})
			}
			ids = append(ids, id)
			digests = append(digests, id.Digest)
		}
		return ids, digests
	}
	depSeen := make(map[string]depSpec)
	var wheelCount int
	var depTruncated bool
	hasInput := len(reqs) > 0

	seen := make(map[string]bool)
	// Seed from requirements.txt
	for _, r := range reqs {
		name := normalizeName(r.Name)
		if name == "" {
			continue
		}
		if len(depSeen) >= opts.MaxDeps {
			depTruncated = true
			break
		}
		if existing, ok := depSeen[name]; ok && existing.Version != "" {
			continue
		}
		version := r.Version
		if cv, ok := constraints[name]; ok && cv != "" {
			version = cv
		}
		if ov, ok := opts.PackageOverrides[name]; ok && ov != "" {
			version = strings.TrimPrefix(strings.TrimSpace(ov), "==")
		}
		if (version == "" || strings.HasPrefix(version, ">=") || strings.HasPrefix(version, "~=")) && resolver != nil {
			if ver, err := resolver.ResolveLatest(name); err == nil {
				version = ver
			} else {
				log.Printf("warn: resolve latest for %s failed: %v", name, err)
			}
		}
		if version == "" {
			version = "latest"
		}
		depSeen[name] = depSpec{Name: name, Version: version}
		key := name + "::" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		packIDs, packDigests := selectPacks(name)
		nodes = append(nodes, FlatNode{
			Name:          name,
			Version:       version,
			PythonVersion: pythonVersion,
			PythonTag:     pyTag,
			PlatformTag:   platformTag,
			Action:        "build",
		})
		wheelKey := artifact.WheelKey{
			SourceDigest:  sourceDigest(name, version),
			PyTag:         pyTag,
			PlatformTag:   platformTag,
			RuntimeDigest: rtID.Digest,
			PackDigests:   packDigests,
		}
		wheelID := artifact.ID{Type: artifact.WheelType, Digest: wheelKey.Digest()}
		wheelAction := "build"
		if ok, _ := store.Has(ctx, wheelID); ok {
			wheelAction = "reuse"
		}
		dagNodes = append(dagNodes, DAGNode{
			ID:     wheelID,
			Type:   NodeWheel,
			Inputs: append([]artifact.ID{rtID}, packIDs...),
			Metadata: map[string]any{
				"name":           name,
				"version":        version,
				"python_version": pythonVersion,
				"python_tag":     pyTag,
				"platform_tag":   platformTag,
			},
			Action: wheelAction,
		})
		addRepair(wheelID, map[string]any{"wheel_name": name, "wheel_version": version})
	}

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".whl") {
			continue
		}
		info, err := parseWheelFilename(f.Name())
		if err != nil {
			continue
		}
		hasInput = true
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
				} else {
					log.Printf("warn: resolve latest for %s failed: %v", dep.Name, err)
				}
			}
			depSeen[dep.Name] = dep
		}

		if overrideVer, ok := opts.PackageOverrides[normalizeName(info.Name)]; ok && overrideVer != "" {
			ver := strings.TrimPrefix(strings.TrimSpace(overrideVer), "==")
			key := info.Name + "::" + ver
			if !seen[key] {
				seen[key] = true
				packIDs, packDigests := selectPacks(info.Name)
				nodes = append(nodes, FlatNode{
					Name:          info.Name,
					Version:       ver,
					PythonVersion: pythonVersion,
					PythonTag:     pyTag,
					PlatformTag:   platformTag,
					Action:        "build",
				})
				wk := artifact.WheelKey{
					SourceDigest:  sourceDigest(info.Name, ver),
					PyTag:         pyTag,
					PlatformTag:   platformTag,
					RuntimeDigest: rtID.Digest,
					PackDigests:   packDigests,
				}
				wID := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}
				wheelAction := "build"
				if ok, _ := store.Has(ctx, wID); ok {
					wheelAction = "reuse"
				}
				dagNodes = append(dagNodes, DAGNode{
					ID:     wID,
					Type:   NodeWheel,
					Inputs: append([]artifact.ID{rtID}, packIDs...),
					Metadata: map[string]any{
						"name":           info.Name,
						"version":        ver,
						"python_version": pythonVersion,
						"python_tag":     pyTag,
						"platform_tag":   platformTag,
					},
					Action: wheelAction,
				})
				addRepair(wID, map[string]any{"wheel_name": info.Name, "wheel_version": ver})
			}
			continue
		}
		key := info.Name + "::" + info.Version
		if seen[key] {
			continue
		}
		seen[key] = true

		packIDs, packDigests := selectPacks(info.Name)
		if isCompatible(info, pyTag, platformTag) {
			nodes = append(nodes, FlatNode{
				Name:          info.Name,
				Version:       info.Version,
				PythonVersion: pythonVersion,
				PythonTag:     pyTag,
				PlatformTag:   platformTag,
				Action:        "reuse",
			})
			wk := artifact.WheelKey{SourceDigest: bestSourceDigest(info.Name, info.Version, filepath.Join(inputDir, f.Name())), PyTag: pyTag, PlatformTag: platformTag, RuntimeDigest: rtID.Digest, PackDigests: packDigests}
			wID := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}
			wheelAction := "reuse"
			if ok, _ := store.Has(ctx, wID); ok {
				wheelAction = "reuse"
			}
			dagNodes = append(dagNodes, DAGNode{
				ID:     wID,
				Type:   NodeWheel,
				Inputs: append([]artifact.ID{rtID}, packIDs...),
				Metadata: map[string]any{
					"name":           info.Name,
					"version":        info.Version,
					"python_version": pythonVersion,
					"python_tag":     pyTag,
					"platform_tag":   platformTag,
				},
				Action: wheelAction,
			})
			addRepair(wID, map[string]any{"wheel_name": info.Name, "wheel_version": info.Version})
		} else {
			nodes = append(nodes, FlatNode{
				Name:          info.Name,
				Version:       info.Version,
				PythonVersion: pythonVersion,
				PythonTag:     pyTag,
				PlatformTag:   platformTag,
				Action:        "build",
			})
			wk := artifact.WheelKey{SourceDigest: bestSourceDigest(info.Name, info.Version, filepath.Join(inputDir, f.Name())), PyTag: pyTag, PlatformTag: platformTag, RuntimeDigest: rtID.Digest, PackDigests: packDigests}
			wID := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}
			wheelAction := "build"
			if ok, _ := store.Has(ctx, wID); ok {
				wheelAction = "reuse"
			}
			dagNodes = append(dagNodes, DAGNode{
				ID:     wID,
				Type:   NodeWheel,
				Inputs: append([]artifact.ID{rtID}, packIDs...),
				Metadata: map[string]any{
					"name":           info.Name,
					"version":        info.Version,
					"python_version": pythonVersion,
					"python_tag":     pyTag,
					"platform_tag":   platformTag,
				},
				Action: wheelAction,
			})
			addRepair(wID, map[string]any{"wheel_name": info.Name, "wheel_version": info.Version})
		}
	}
	for dep, spec := range depSeen {
		if dep == "" {
			continue
		}
		version := spec.Version
		if ov, ok := opts.PackageOverrides[normalizeName(dep)]; ok && ov != "" {
			version = strings.TrimPrefix(strings.TrimSpace(ov), "==")
		}
		if cv, ok := constraints[normalizeName(dep)]; ok && cv != "" {
			version = cv
		}
		if version == "" {
			if resolver != nil {
				if ver, err := resolver.ResolveLatest(dep); err == nil {
					version = ver
				} else {
					log.Printf("warn: resolve latest for %s failed: %v", dep, err)
				}
			}
			if version == "" {
				version = "latest"
			}
		}
		if opts.UpgradeStrategy == "eager" && resolver != nil {
			if ver, err := resolver.ResolveLatest(dep); err == nil && ver != "" {
				version = ver
			} else if err != nil {
				log.Printf("warn: eager resolve latest for %s failed: %v", dep, err)
			}
		}
		key := dep + "::" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		packIDs, packDigests := selectPacks(dep)
		nodes = append(nodes, FlatNode{
			Name:          dep,
			Version:       version,
			PythonVersion: pythonVersion,
			PythonTag:     pyTag,
			PlatformTag:   platformTag,
			Action:        "build",
		})
		wk := artifact.WheelKey{SourceDigest: sourceDigest(dep, version), PyTag: pyTag, PlatformTag: platformTag, RuntimeDigest: rtID.Digest, PackDigests: packDigests}
		wID := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}
		wheelAction := "build"
		if ok, _ := store.Has(ctx, wID); ok {
			wheelAction = "reuse"
		}
		dagNodes = append(dagNodes, DAGNode{
			ID:     wID,
			Type:   NodeWheel,
			Inputs: append([]artifact.ID{rtID}, packIDs...),
			Metadata: map[string]any{
				"name":           dep,
				"version":        version,
				"python_version": pythonVersion,
				"python_tag":     pyTag,
				"platform_tag":   platformTag,
			},
			Action: wheelAction,
		})
		addRepair(wID, map[string]any{"wheel_name": dep, "wheel_version": version})
	}
	if !hasInput {
		return Snapshot{}, fmt.Errorf("no wheels or requirements found in input directory %s", inputDir)
	}
	if depTruncated {
		return Snapshot{}, fmt.Errorf("dependency expansion exceeded MaxDeps (%d); increase MAX_DEPS or trim input", opts.MaxDeps)
	}
	return Snapshot{RunID: newRunID(), Plan: nodes, DAG: dagNodes}, nil
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

func sourceDigest(name, version string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s==%s", normalizeName(name), strings.TrimSpace(version))))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fileDigest(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func bestSourceDigest(name, version, path string) string {
	if d := fileDigest(path); d != "" {
		return d
	}
	return sourceDigest(name, version)
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

func loadMaxDepsFromEnv() int {
	raw := os.Getenv("MAX_DEPS")
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func loadRequirements(inputDir, path string) []depSpec {
	var reqPath string
	if path != "" {
		reqPath = path
	} else {
		reqPath = filepath.Join(inputDir, "requirements.txt")
	}
	if _, err := os.Stat(reqPath); err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Clean(reqPath))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var out []depSpec
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-c ") || strings.HasPrefix(line, "--constraint") {
			continue
		}
		// strip inline comments
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		name := line
		version := ""
		for _, op := range []string{"==", ">=", "~="} {
			if idx := strings.Index(line, op); idx > 0 {
				name = strings.TrimSpace(line[:idx])
				version = line[idx:]
				if op == "==" {
					version = strings.TrimPrefix(version, "==")
				}
				break
			}
		}
		name = normalizeName(name)
		if name == "" {
			continue
		}
		out = append(out, depSpec{Name: name, Version: version})
	}
	return out
}

func loadConstraints(path string) map[string]string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	out := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name := line
		version := ""
		for _, op := range []string{"==", ">=", "~="} {
			if idx := strings.Index(line, op); idx > 0 {
				name = strings.TrimSpace(line[:idx])
				version = strings.TrimPrefix(line[idx:], "==")
				break
			}
		}
		name = normalizeName(name)
		if name == "" {
			continue
		}
		out[name] = version
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

// packDependencies declares manual pack dependency edges to enforce ordering.
func packDependencies(name string) []string {
	deps := map[string][]string{
		"openssl":       {"zlib"},
		"libpng":        {"zlib"},
		"freetype":      {"libpng", "jpeg"},
		"libxslt":       {"libxml2"},
		"libxml2":       {"zlib"},
		"cpython":       {"openssl", "libffi", "zlib", "xz", "bzip2", "sqlite"},
		"cpython3.10":   {"openssl", "libffi", "zlib", "xz", "bzip2", "sqlite"},
		"cpython3.11":   {"openssl", "libffi", "zlib", "xz", "bzip2", "sqlite"},
		"cpython3.12":   {"openssl", "libffi", "zlib", "xz", "bzip2", "sqlite"},
		"runtime":       {"openssl", "libffi", "zlib", "xz", "bzip2", "sqlite"},
		"libjpeg-turbo": {},
	}
	return deps[strings.ToLower(name)]
}
