package plan

import (
	"os"
	"path/filepath"
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

	snap, err := compute(dir, "3.11", "manylinux2014_s390x")
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

func TestLoadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	snap := Snapshot{RunID: "abc", Plan: []Node{{Name: "pkg", Version: "1.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}}}
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
	meta := "Metadata-Version: 2.1\nName: demo\nRequires-Dist: depA (>=1.0)\nRequires-Dist: depB\n"
	reqs := parseRequiresDist(meta)
	if len(reqs) != 2 || reqs[0] != "depa" || reqs[1] != "depb" {
		t.Fatalf("unexpected requires-dist parse: %+v", reqs)
	}
}

func TestGenerateWritesPlan(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "pkg-0.1.0-py3-none-any.whl"), []byte{}, 0o644)
	_, err := Generate(dir, planDir, "3.11", "manylinux2014_s390x")
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
	snap, err := compute(wheelDir, "3.11", "manylinux2014_s390x")
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
