package builder

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPackArchivesRecipeOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.tar")
	err := BuildPack(path, PackBuildOpts{
		Digest: "sha256:pack",
		Cmd: `mkdir -p "$PACK_OUTPUT/usr/local/include" && \
printf 'stub\n' > "$PACK_OUTPUT/usr/local/include/stub.h" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"stub","version":"1.0.0"}
EOF`,
	})
	if err != nil {
		t.Fatalf("BuildPack: %v", err)
	}
	names := tarEntries(t, path)
	requireTarEntry(t, names, "manifest.json")
	requireTarEntry(t, names, "usr/local/include/stub.h")
}

func TestBuildRuntimeRequiresInterpreter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.tar")
	err := BuildRuntime(path, RuntimeBuildOpts{
		Digest:        "sha256:rt",
		PythonVersion: "3.11",
		Cmd: `mkdir -p "$PACK_OUTPUT/usr/local/bin" && \
printf '#!/bin/sh\nexit 0\n' > "$PACK_OUTPUT/usr/local/bin/python3" && \
chmod +x "$PACK_OUTPUT/usr/local/bin/python3" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"cpython311","version":"3.11.0"}
EOF`,
	})
	if err != nil {
		t.Fatalf("BuildRuntime: %v", err)
	}
	names := tarEntries(t, path)
	requireTarEntry(t, names, "manifest.json")
	requireTarEntry(t, names, "usr/local/bin/python3")
}

func TestBuildRuntimeRejectsManifestOnlyOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.tar")
	err := BuildRuntime(path, RuntimeBuildOpts{
		Digest:        "sha256:rt",
		PythonVersion: "3.11",
		Cmd: `cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"cpython311","version":"3.11.0"}
EOF`,
	})
	if err == nil {
		t.Fatalf("expected BuildRuntime to reject manifest-only output")
	}
}

func TestBuildRuntimePreservesSymlinkTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.tar")
	err := BuildRuntime(path, RuntimeBuildOpts{
		Digest:        "sha256:rt",
		PythonVersion: "3.11",
		Cmd: `mkdir -p "$PACK_OUTPUT/usr/local/bin" && \
printf '#!/bin/sh\nexit 0\n' > "$PACK_OUTPUT/usr/local/bin/python3.11" && \
chmod +x "$PACK_OUTPUT/usr/local/bin/python3.11" && \
ln -s python3.11 "$PACK_OUTPUT/usr/local/bin/python3" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"cpython311","version":"3.11.0"}
EOF`,
	})
	if err != nil {
		t.Fatalf("BuildRuntime: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tar: %v", err)
	}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("missing python3 symlink entry")
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		if hdr.Name != "usr/local/bin/python3" {
			continue
		}
		if hdr.Typeflag != tar.TypeSymlink {
			t.Fatalf("expected symlink entry, got type %q", hdr.Typeflag)
		}
		if hdr.Linkname != "python3.11" {
			t.Fatalf("unexpected symlink target %q", hdr.Linkname)
		}
		return
	}
}

func tarEntries(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tar: %v", err)
	}
	tr := tar.NewReader(bytes.NewReader(data))
	out := make(map[string]struct{})
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		out[hdr.Name] = struct{}{}
	}
}

func requireTarEntry(t *testing.T, names map[string]struct{}, want string) {
	t.Helper()
	if _, ok := names[want]; !ok {
		t.Fatalf("missing tar entry %s", want)
	}
}
