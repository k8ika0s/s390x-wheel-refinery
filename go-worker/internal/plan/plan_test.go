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
