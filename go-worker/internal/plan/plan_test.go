package plan

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/cas"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWheelFilename(t *testing.T) {
	info, err := parseWheelFilename("pkgA-1.2.3-cp311-cp311-manylinux2014_s390x.whl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "pkgA" || info.Version != "1.2.3" || info.PythonTag != "cp311" || info.PlatformTag != "manylinux2014_s390x" {
		t.Fatalf("unexpected parse result: %+v", info)
	}
}

func TestNormalizePyTag(t *testing.T) {
	tests := map[string]string{
		"3.11":    "cp311",
		"311":     "cp311",
		"cp311":   "cp311",
		"py3":     "cppy3", // unusual, but verify prefix handling
		"3.10":    "cp310",
		"py311":   "cppy311",
		"cp38":    "cp38",
		"3":       "cp3",
		"pypy3":   "cppypy3",
		"3.12.1":  "cp3121",
		"3.13a1":  "cp313a1",
		"311-dev": "cp311-dev",
	}
	for in, want := range tests {
		got := normalizePyTag(in)
		if got != want {
			t.Fatalf("normalizePyTag(%q)=%q want %q", in, got, want)
		}
	}
}

func TestComputePlanReuseVsBuild(t *testing.T) {
	dir := t.TempDir()
	// reusable pure wheel
	os.WriteFile(filepath.Join(dir, "purepkg-1.0.0-py3-none-any.whl"), []byte{}, 0o644)
	// platform-specific incompatible
	os.WriteFile(filepath.Join(dir, "nativepkg-2.0.0-cp311-cp311-manylinux_x86_64.whl"), []byte{}, 0o644)

	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned"}, nil)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	if len(snap.Plan) != 2 {
		t.Fatalf("expected 2 plan nodes, got %d", len(snap.Plan))
	}
	var reuse, build int
	for _, n := range snap.Plan {
		if n.Action == "reuse" {
			reuse++
		} else if n.Action == "build" {
			build++
		}
	}
	if reuse != 1 || build != 1 {
		t.Fatalf("expected 1 reuse and 1 build, got reuse=%d build=%d", reuse, build)
	}
}

func TestComputePlanIncludesDAG(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo-1.0.0-py3-none-any.whl"), []byte{}, 0o644)

	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned"}, nil)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	if len(snap.DAG) == 0 {
		t.Fatalf("expected DAG nodes")
	}
	rtKey := artifact.RuntimeKey{Arch: "s390x", PolicyBaseDigest: "", PythonVersion: "3.11"}
	rtDigest := rtKey.Digest()
	wk := artifact.WheelKey{
		SourceDigest:  sourceDigest("demo", "1.0.0"),
		PyTag:         "cp311",
		PlatformTag:   "manylinux2014_s390x",
		RuntimeDigest: rtDigest,
	}
	wantWheelID := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}

	var runtimeFound, wheelFound bool
	for _, n := range snap.DAG {
		switch n.Type {
		case NodeRuntime:
			if n.ID.Digest == rtDigest {
				runtimeFound = true
			}
		case NodeWheel:
			if n.ID == wantWheelID {
				wheelFound = true
				if len(n.Inputs) != 1 || n.Inputs[0].Digest != rtDigest {
					t.Fatalf("wheel node missing runtime input: %+v", n.Inputs)
				}
			}
		}
	}
	if !runtimeFound {
		t.Fatalf("runtime node missing from DAG")
	}
	if !wheelFound {
		t.Fatalf("wheel node missing from DAG")
	}
}

func TestPackCatalogAddsPackNodesToDAG(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "requirements.txt")
	if err := os.WriteFile(reqPath, []byte("demo==1.0.0"), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}
	cat := &pack.Catalog{
		Packs: map[string]pack.PackDef{
			"openssl": {Name: "openssl", Version: "3.0", RecipeDigest: "sha256:abc"},
		},
		Rules: []pack.Rule{
			{PackagePattern: "demo", Packs: []string{"openssl"}},
		},
	}
	opts := Options{UpgradeStrategy: "pinned", RequirementsPath: reqPath, PackCatalog: cat}
	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", opts, nil)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	packKey := artifact.PackKey{Arch: "s390x", PolicyBaseDigest: "", Name: "openssl", Version: "3.0", RecipeDigest: "sha256:abc"}
	packID := artifact.ID{Type: artifact.PackType, Digest: packKey.Digest()}
	rtKey := artifact.RuntimeKey{Arch: "s390x", PolicyBaseDigest: "", PythonVersion: "3.11"}
	rtDigest := rtKey.Digest()
	wk := artifact.WheelKey{
		SourceDigest:  sourceDigest("demo", "1.0.0"),
		PyTag:         "cp311",
		PlatformTag:   "manylinux2014_s390x",
		RuntimeDigest: rtDigest,
		PackDigests:   []string{packID.Digest},
	}
	wantWheel := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}

	var sawPack, sawWheel bool
	for _, n := range snap.DAG {
		if n.ID == packID {
			sawPack = true
		}
		if n.ID == wantWheel {
			sawWheel = true
			var foundPack bool
			for _, inp := range n.Inputs {
				if inp == packID {
					foundPack = true
				}
			}
			if !foundPack {
				t.Fatalf("wheel node missing pack input: %+v", n.Inputs)
			}
		}
	}
	if !sawPack {
		t.Fatalf("pack node not present in DAG")
	}
	if !sawWheel {
		t.Fatalf("wheel node with pack input not present")
	}
}

func TestRepairNodesFollowWheels(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "requirements.txt")
	if err := os.WriteFile(reqPath, []byte("demo==1.0.0"), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}
	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned", RequirementsPath: reqPath}, nil)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	var wheels []artifact.ID
	var repairs []DAGNode
	for _, n := range snap.DAG {
		if n.Type == NodeWheel {
			wheels = append(wheels, n.ID)
		}
		if n.Type == NodeRepair {
			repairs = append(repairs, n)
		}
	}
	if len(repairs) != len(wheels) {
		t.Fatalf("expected %d repair nodes, got %d", len(wheels), len(repairs))
	}
	for _, r := range repairs {
		if len(r.Inputs) != 1 {
			t.Fatalf("repair node should have one input: %+v", r.Inputs)
		}
		found := false
		for _, w := range wheels {
			if r.Inputs[0] == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("repair node input not a wheel: %+v", r.Inputs[0])
		}
	}
}

func TestCASOverridesActionsToReuse(t *testing.T) {
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "requirements.txt")
	if err := os.WriteFile(reqPath, []byte("demo==1.0.0"), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}
	cat := &pack.Catalog{
		Packs: map[string]pack.PackDef{
			"openssl": {Name: "openssl", Version: "3.0"},
		},
		Rules: []pack.Rule{{PackagePattern: "demo", Packs: []string{"openssl"}}},
	}
	rtKey := artifact.RuntimeKey{Arch: "s390x", PolicyBaseDigest: "", PythonVersion: "3.11"}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: rtKey.Digest()}
	packKey := artifact.PackKey{Arch: "s390x", PolicyBaseDigest: "", Name: "openssl", Version: "3.0"}
	packID := artifact.ID{Type: artifact.PackType, Digest: packKey.Digest()}
	wk := artifact.WheelKey{
		SourceDigest:  sourceDigest("demo", "1.0.0"),
		PyTag:         "cp311",
		PlatformTag:   "manylinux2014_s390x",
		RuntimeDigest: rtID.Digest,
		PackDigests:   []string{packID.Digest},
	}
	wID := artifact.ID{Type: artifact.WheelType, Digest: wk.Digest()}
	repairKey := artifact.RepairKey{InputWheelDigest: wID.Digest}
	repairID := artifact.ID{Type: artifact.RepairType, Digest: repairKey.Digest()}

	store := cas.NewMemoryStore()
	store.Add(rtID)
	store.Add(packID)
	store.Add(wID)
	store.Add(repairID)

	opts := Options{UpgradeStrategy: "pinned", RequirementsPath: reqPath, PackCatalog: cat, ArtifactStore: store}
	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", opts, nil)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}

	find := func(id artifact.ID) (DAGNode, bool) {
		for _, n := range snap.DAG {
			if n.ID == id {
				return n, true
			}
		}
		return DAGNode{}, false
	}
	if n, ok := find(rtID); !ok || n.Action != "reuse" {
		t.Fatalf("runtime not marked reuse: %+v", n)
	}
	if n, ok := find(packID); !ok || n.Action != "reuse" {
		t.Fatalf("pack not marked reuse: %+v", n)
	}
	if n, ok := find(wID); !ok || n.Action != "reuse" {
		t.Fatalf("wheel not marked reuse: %+v", n)
	}
	if n, ok := find(repairID); !ok || n.Action != "reuse" {
		t.Fatalf("repair not marked reuse: %+v", n)
	}
}

func TestLoadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	snap := Snapshot{RunID: "abc", Plan: []FlatNode{{Name: "pkg", Version: "1.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}}}
	if err := Write(path, snap); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.RunID != snap.RunID || len(got.Plan) != 1 || got.Plan[0].Name != "pkg" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestIsCompatible(t *testing.T) {
	ok := isCompatible(wheelInfo{Name: "p", Version: "1", PythonTag: "cp311", AbiTag: "cp311", PlatformTag: "manylinux2014_s390x"}, "cp311", "manylinux2014_s390x")
	if !ok {
		t.Fatalf("expected compatible")
	}
	not := isCompatible(wheelInfo{Name: "p", Version: "1", PythonTag: "cp311", AbiTag: "cp311", PlatformTag: "manylinux_x86_64"}, "cp311", "manylinux2014_s390x")
	if not {
		t.Fatalf("expected incompatible platform")
	}
}

func TestParseRequiresDist(t *testing.T) {
	meta := "Metadata-Version: 2.1\nName: demo\nRequires-Dist: depA (>=1.0)\nRequires-Dist: depB\nRequires-Dist: depC (==2.3.4)\n"
	reqs := parseRequiresDist(meta)
	if len(reqs) != 3 || reqs[0].Name != "depa" || reqs[1].Name != "depb" || reqs[2].Version != "==2.3.4" {
		t.Fatalf("unexpected requires-dist parse: %+v", reqs)
	}
}

func TestParseRequiresDistExtrasAndMarkers(t *testing.T) {
	meta := "Requires-Dist: Fancy_Pkg[extra] (>=2.0); python_version >= \"3.8\"\nRequires-Dist: otherpkg~=1.4"
	reqs := parseRequiresDist(meta)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(reqs))
	}
	if reqs[0].Name != "fancy-pkg" || reqs[0].Version != ">=2.0" {
		t.Fatalf("unexpected first dep: %+v", reqs[0])
	}
	if reqs[1].Name != "otherpkg" || reqs[1].Version != "~=1.4" {
		t.Fatalf("unexpected second dep: %+v", reqs[1])
	}
}

func TestGenerateWritesPlan(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "pkg-0.1.0-py3-none-any.whl"), []byte{}, 0o644)
	_, err := Generate(dir, planDir, "3.11", "manylinux2014_s390x", "", "", "pinned", "", "")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planDir, "plan.json")); err != nil {
		t.Fatalf("plan.json not written: %v", err)
	}
}

func TestComputeFixtureMatchesExpected(t *testing.T) {
	fixtureDir := filepath.Join("testdata")
	wheelDir := filepath.Join(fixtureDir, "wheels")
	snap, err := computeWithResolver(wheelDir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned"}, nil)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	expectedPath := filepath.Join(fixtureDir, "expected_plan.json")
	expSnap, err := Load(expectedPath)
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}
	if len(snap.Plan) != len(expSnap.Plan) {
		t.Fatalf("plan length mismatch: got %d want %d", len(snap.Plan), len(expSnap.Plan))
	}
	for _, exp := range expSnap.Plan {
		found := false
		for _, node := range snap.Plan {
			if exp.Name == node.Name && exp.Version == node.Version && exp.Action == node.Action {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected node not found: %+v", exp)
		}
	}
}

func TestJSONShapeMatchesPythonSnapshot(t *testing.T) {
	snap := Snapshot{
		RunID: "abc",
		Plan: []FlatNode{
			{Name: "pkg", Version: "1.0.0", PythonTag: "cp311", PythonVersion: "3.11", PlatformTag: "manylinux2014_s390x", Action: "build"},
		},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["run_id"]; !ok {
		t.Fatalf("run_id missing")
	}
	planVal, ok := decoded["plan"]
	if !ok {
		t.Fatalf("plan missing")
	}
	arr, ok := planVal.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("plan array wrong: %#v", planVal)
	}
	node, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("node not object: %#v", arr[0])
	}
	required := []string{"name", "version", "python_tag", "python_version", "platform_tag", "action"}
	for _, k := range required {
		if _, ok := node[k]; !ok {
			t.Fatalf("missing field %s", k)
		}
	}
	if len(node) != len(required) {
		t.Fatalf("unexpected extra fields in node: %#v", node)
	}
}

func BenchmarkComputeFixture(b *testing.B) {
	fixtureDir := filepath.Join("testdata")
	wheelDir := filepath.Join(fixtureDir, "wheels")
	for i := 0; i < b.N; i++ {
		_, err := computeWithResolver(wheelDir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned"}, nil)
		if err != nil {
			b.Fatalf("compute failed: %v", err)
		}
	}
}

type mockResolver struct {
	versions map[string]string
}

func (m *mockResolver) ResolveLatest(name string) (string, error) {
	if v, ok := m.versions[name]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}

func writeWheelWithMeta(t *testing.T, dir, name, meta string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	zf, err := os.Create(path)
	if err != nil {
		t.Fatalf("create wheel: %v", err)
	}
	zipw := zip.NewWriter(zf)
	metaFile, err := zipw.Create("demo-0.1.0.dist-info/METADATA")
	if err != nil {
		t.Fatalf("create meta: %v", err)
	}
	if _, err := metaFile.Write([]byte(meta)); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if err := zipw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := zf.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	return path
}

func TestUpgradeStrategyEagerOverridesPins(t *testing.T) {
	dir := t.TempDir()
	meta := "Metadata-Version: 2.1\nName: demo\nRequires-Dist: depA (==1.0.0)\nRequires-Dist: depB\n"
	writeWheelWithMeta(t, dir, "demo-0.1.0-py3-none-any.whl", meta)

	resolver := &mockResolver{versions: map[string]string{"depa": "9.9.9", "depb": "2.0.0"}}

	pinnedSnap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned"}, resolver)
	if err != nil {
		t.Fatalf("pinned compute: %v", err)
	}
	eagerSnap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "eager"}, resolver)
	if err != nil {
		t.Fatalf("eager compute: %v", err)
	}

	find := func(plan Snapshot, name string) FlatNode {
		for _, n := range plan.Plan {
			if n.Name == name {
				return n
			}
		}
		return FlatNode{}
	}

	if find(pinnedSnap, "depa").Version != "==1.0.0" {
		t.Fatalf("pinned should keep depA pin, got %s", find(pinnedSnap, "depa").Version)
	}
	if find(pinnedSnap, "depb").Version != "2.0.0" {
		t.Fatalf("pinned should resolve depB latest, got %s", find(pinnedSnap, "depb").Version)
	}
	if find(eagerSnap, "depa").Version != "9.9.9" {
		t.Fatalf("eager should upgrade depA, got %s", find(eagerSnap, "depa").Version)
	}
	if find(eagerSnap, "depb").Version != "2.0.0" {
		t.Fatalf("eager should use latest for depB, got %s", find(eagerSnap, "depb").Version)
	}
}

func TestMissingVersionFallsBackToLatestOnResolverFailure(t *testing.T) {
	dir := t.TempDir()
	meta := "Requires-Dist: missingdep"
	writeWheelWithMeta(t, dir, "demo-0.1.0-py3-none-any.whl", meta)

	resolver := &mockResolver{versions: map[string]string{}}
	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", Options{UpgradeStrategy: "pinned"}, resolver)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	found := false
	for _, n := range snap.Plan {
		if n.Name == "missingdep" {
			found = true
			if n.Version != "latest" {
				t.Fatalf("expected fallback latest, got %s", n.Version)
			}
		}
	}
	if !found {
		t.Fatalf("missingdep not planned")
	}
}

func TestMaxDepsLimit(t *testing.T) {
	dir := t.TempDir()
	var metaLines []string
	for i := 0; i < 50; i++ {
		metaLines = append(metaLines, fmt.Sprintf("Requires-Dist: dep%d", i))
	}
	meta := strings.Join(metaLines, "\n")
	writeWheelWithMeta(t, dir, "demo-0.1.0-py3-none-any.whl", meta)

	opts := Options{UpgradeStrategy: "pinned", MaxDeps: 10}
	_, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", opts, &mockResolver{})
	if err == nil || !strings.Contains(err.Error(), "MaxDeps") {
		t.Fatalf("expected MaxDeps error, got %v", err)
	}
}

func TestPackageOverridesApplyToTopLevelAndDeps(t *testing.T) {
	dir := t.TempDir()
	meta := "Requires-Dist: depA\n"
	writeWheelWithMeta(t, dir, "demo-0.1.0-py3-none-any.whl", meta)

	overrides := map[string]string{
		"demo": "9.9.9",
		"depa": "8.8.8",
	}
	opts := Options{UpgradeStrategy: "pinned", PackageOverrides: overrides}
	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", opts, &mockResolver{})
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	find := func(name string) (FlatNode, bool) {
		for _, n := range snap.Plan {
			if n.Name == name {
				return n, true
			}
		}
		return FlatNode{}, false
	}
	if n, ok := find("demo"); !ok || n.Version != "9.9.9" || n.Action != "build" {
		t.Fatalf("override for demo not applied: %+v", n)
	}
	if n, ok := find("depa"); !ok || n.Version != "8.8.8" {
		t.Fatalf("override for depA not applied: %+v", n)
	}
}

func TestRequirementsOnlyGeneratesPlan(t *testing.T) {
	dir := t.TempDir()
	req := "foo==1.2.3\nbar>=2.0\n# comment\nbaz"
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(req), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}
	resolver := &mockResolver{versions: map[string]string{"bar": "2.1.0", "baz": "3.0.0"}}
	opts := Options{UpgradeStrategy: "pinned", RequirementsPath: filepath.Join(dir, "requirements.txt")}
	snap, err := computeWithResolver(dir, "3.11", "manylinux2014_s390x", opts, resolver)
	if err != nil {
		t.Fatalf("compute failed: %v", err)
	}
	if len(snap.Plan) != 3 {
		t.Fatalf("expected 3 plan nodes, got %d", len(snap.Plan))
	}
	want := map[string]string{"foo": "1.2.3", "bar": "2.1.0", "baz": "3.0.0"}
	for _, n := range snap.Plan {
		if n.Version != want[n.Name] {
			t.Fatalf("unexpected version for %s: %s", n.Name, n.Version)
		}
		if n.Action != "build" {
			t.Fatalf("expected build action for %s", n.Name)
		}
	}
}
