package service

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/builder"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/cas"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

type fakeStore struct {
	keys []string
}

func (f *fakeStore) Put(_ context.Context, key string, _ []byte, _ string) error {
	f.keys = append(f.keys, key)
	return nil
}

func (f *fakeStore) Get(_ context.Context, _ string) ([]byte, string, error) {
	return nil, "", io.EOF
}

func (f *fakeStore) URL(key string) string {
	return "http://minio/" + key
}

func TestUploadArtifactsFiltersWheels(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	// matching wheel
	if err := os.WriteFile(filepath.Join(output, "demo-1.0.0-py3-none-any.whl"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-matching wheel
	if err := os.WriteFile(filepath.Join(output, "other-1.0.0-py3-none-any.whl"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{}
	w := &Worker{
		Cfg:   Config{OutputDir: output},
		Store: fs,
	}
	w.uploadArtifacts(context.Background(), runner.Job{Name: "demo", Version: "1.0.0"})
	if len(fs.keys) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(fs.keys))
	}
	if fs.keys[0] != "demo/1.0.0/demo-1.0.0-py3-none-any.whl" {
		t.Fatalf("unexpected key: %s", fs.keys[0])
	}
}

func TestUploadArtifactsPublishesRuntimeFromLocalCASTar(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out")
	localCAS := filepath.Join(dir, "cas")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(localCAS, 0o755); err != nil {
		t.Fatal(err)
	}

	var manifestPuts atomic.Int32
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/artifacts/blobs/uploads/":
			w.Header().Set("Location", "/upload/test")
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/upload/test"):
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v2/artifacts/manifests/"):
			manifestPuts.Add(1)
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	tarBuf, runtimeDigest := sampleRuntimeTarWithDigest()
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: runtimeDigest}
	archivePath := filepath.Join(localCAS, strings.ReplaceAll(runtimeDigest, ":", "_")+".tar")
	if err := os.WriteFile(archivePath, tarBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(localCAS, strings.ReplaceAll(runtimeDigest, ":", "_"), "usr", "local")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	w := &Worker{
		Cfg: Config{
			OutputDir:          output,
			LocalCASDir:        localCAS,
			RuntimePushEnabled: true,
			CASRegistryURL:     registry.URL,
			CASRegistryRepo:    "artifacts",
		},
		Pusher: cas.Pusher{
			BaseURL: registry.URL,
			Repo:    "artifacts",
			Client:  registry.Client(),
		},
	}
	uploads := w.uploadArtifacts(context.Background(), runner.Job{
		Name:          "demo",
		Version:       "1.0.0",
		RuntimeDigest: rtID.Digest,
		RuntimePath:   runtimeDir,
	})
	if uploads.RuntimeURL != registry.URL+"/v2/artifacts/manifests/"+strings.ReplaceAll(runtimeDigest, ":", "-") {
		t.Fatalf("unexpected runtime url: %q", uploads.RuntimeURL)
	}
	if manifestPuts.Load() == 0 {
		t.Fatalf("expected runtime manifest push")
	}
}

func TestUploadArtifactsSkipsRepairMetadataWhenRepairNotProduced(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	wheelPath := filepath.Join(output, "demo-1.0.0-py3-none-any.whl")
	if err := os.WriteFile(wheelPath, []byte("wheel"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &Worker{
		Cfg: Config{
			OutputDir:         output,
			RepairPushEnabled: true,
			CASRegistryURL:    "http://registry.invalid",
			CASRegistryRepo:   "artifacts",
			RepairCmd:         "exit 1",
		},
		Pusher: cas.Pusher{BaseURL: "http://registry.invalid", Repo: "artifacts"},
	}
	uploads := w.uploadArtifacts(context.Background(), runner.Job{
		Name:        "demo",
		Version:     "1.0.0",
		WheelDigest: "sha256:wheel",
	})
	if uploads.RepairDigest != "" || uploads.RepairURL != "" {
		t.Fatalf("expected no repair metadata, got %+v", uploads)
	}
}

func TestFetchArtifactUsesFetcher(t *testing.T) {
	dir := t.TempDir()
	fetched := false
	wheelData := []byte("data")
	wheelDigest := "sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"
	w := &Worker{
		Cfg: Config{CacheDir: dir, LocalCASDir: filepath.Join(dir, "cas")},
		Fetcher: cas.Fetcher{
			BaseURL: "http://example",
			Repo:    "artifacts",
			Client: &http.Client{
				Transport: artifactFetchTransport(func() { fetched = true }, wheelDigest, wheelData, "application/octet-stream"),
			},
		},
		packPath: make(map[string]string),
	}
	job := runner.Job{WheelDigest: wheelDigest, WheelAction: "reuse"}
	if err := w.fetchWheel(context.Background(), job); err != nil {
		t.Fatalf("fetchWheel: %v", err)
	}
	if !fetched {
		t.Fatalf("fetcher not invoked")
	}
}

func TestEnsureContainerImageAvailablePullsMissingImage(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "podman.log")
	bin := filepath.Join(dir, "podman")
	script := `#!/bin/sh
set -eu
echo "$@" >> "` + logPath + `"
if [ "$1" = "image" ] && [ "$2" = "exists" ]; then
  exit 1
fi
if [ "$1" = "pull" ]; then
  exit 0
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Fatal(err)
	}
	w := &Worker{Cfg: Config{PodmanBin: bin, ContainerImage: "127.0.0.1:5000/refinery-builder:latest"}}
	var traces []string
	if err := w.ensureContainerImageAvailable(context.Background(), runner.Job{ContainerImage: "127.0.0.1:5000/refinery-builder:latest"}, func(format string, args ...any) {
		traces = append(traces, fmt.Sprintf(format, args...))
	}); err != nil {
		t.Fatalf("ensureContainerImageAvailable: %v", err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logData)
	if !strings.Contains(got, "image exists 127.0.0.1:5000/refinery-builder:latest") {
		t.Fatalf("expected image exists probe, got %q", got)
	}
	if !strings.Contains(got, "pull --tls-verify=false 127.0.0.1:5000/refinery-builder:latest") {
		t.Fatalf("expected insecure local-registry pull, got %q", got)
	}
	if len(traces) == 0 {
		t.Fatal("expected preflight traces")
	}
}

func TestMatchCarriesFallbackPackRequirementsAndNativeHeavyProfile(t *testing.T) {
	openblasID := packArtifactID(pack.PackDef{Name: "openblas", Version: "0.3.25"})
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{{Name: "scikit-learn", Version: "1.5.2", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}},
		DAG:  []plan.DAGNode{},
	}
	w := &Worker{Cfg: Config{
		ContainerImage:            "refinery-builder:latest",
		ContainerImageNativeHeavy: "refinery-builder-native:latest",
		PackCatalog: &pack.Catalog{
			Packs: map[string]pack.PackDef{
				"openblas": {Name: "openblas", Version: "0.3.25"},
			},
		},
	}}
	reqs := []queue.Request{{
		Package:     "scikit-learn",
		Version:     "1.5.2",
		PythonTag:   "cp311",
		PlatformTag: "manylinux2014_s390x",
		Metadata: map[string]any{
			"builder_profile":   builderProfileNativeHeavy,
			"pack_requirements": []string{"openblas"},
		},
	}}
	jobs := w.match(context.Background(), snap, reqs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].BuilderProfile != builderProfileNativeHeavy {
		t.Fatalf("expected native-heavy profile, got %q", jobs[0].BuilderProfile)
	}
	if jobs[0].ContainerImage != "refinery-builder-native:latest" {
		t.Fatalf("expected native-heavy image, got %q", jobs[0].ContainerImage)
	}
	if got := strings.Join(jobs[0].PackRequirements, ","); got != "openblas" {
		t.Fatalf("expected pack requirements propagated, got %q", got)
	}
	if got := strings.Join(jobs[0].PackDigests, ","); got != openblasID.Digest {
		t.Fatalf("expected fallback pack digest, got %q want %q", got, openblasID.Digest)
	}
}

func TestMatchDefaultsHeavyNativePackagesToNativeHeavyProfile(t *testing.T) {
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{{Name: "scikit-learn", Version: "1.5.2", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}},
		DAG:  []plan.DAGNode{},
	}
	w := &Worker{Cfg: Config{
		ContainerImage:            "refinery-builder:latest",
		ContainerImageNativeHeavy: "refinery-builder-native:latest",
	}}
	reqs := []queue.Request{{
		Package:     "scikit-learn",
		Version:     "1.5.2",
		PythonTag:   "cp311",
		PlatformTag: "manylinux2014_s390x",
	}}
	jobs := w.match(context.Background(), snap, reqs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].BuilderProfile != builderProfileNativeHeavy {
		t.Fatalf("expected native-heavy profile by default, got %q", jobs[0].BuilderProfile)
	}
	if jobs[0].ContainerImage != "refinery-builder-native:latest" {
		t.Fatalf("expected native-heavy image by default, got %q", jobs[0].ContainerImage)
	}
}

func TestExtractTarRestoresRegularFilesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "usr", "local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "usr", "local", "lib", "python3.11", "site-packages", "pip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "manifest.json"), []byte(`{"name":"cpython311"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "usr", "local", "bin", "python3.11"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("python3.11", filepath.Join(srcDir, "usr", "local", "bin", "python3")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "usr", "local", "lib", "python3.11", "site-packages", "pip", "__init__.py"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(dir, "runtime.tar")
	if err := writeTestTarFromDir(tarPath, srcDir); err != nil {
		t.Fatalf("writeTar: %v", err)
	}

	destDir := filepath.Join(dir, "dest")
	if err := extractTar(tarPath, destDir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "usr", "local", "bin", "python3.11")); err != nil {
		t.Fatalf("expected extracted python3.11: %v", err)
	}
	if got, err := os.Readlink(filepath.Join(destDir, "usr", "local", "bin", "python3")); err != nil {
		t.Fatalf("expected extracted python3 symlink: %v", err)
	} else if got != "python3.11" {
		t.Fatalf("unexpected symlink target %q", got)
	}
	if _, err := os.Stat(filepath.Join(destDir, "usr", "local", "lib", "python3.11", "site-packages", "pip", "__init__.py")); err != nil {
		t.Fatalf("expected extracted pip package: %v", err)
	}
}

func writeTestTarFromDir(path, sourceDir string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	defer tw.Close()
	return filepath.Walk(sourceDir, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if current == sourceDir {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(current)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func artifactFetchTransport(onFetch func(), idDigest string, payload []byte, mediaType string) roundTripFunc {
	layerDigest := artifactBlobDigest(payload)
	configDigest := artifactBlobDigest([]byte("{}"))
	ref := strings.ReplaceAll(idDigest, ":", "-")
	manifestPayload := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.unknown.config.v1+json","digest":"%s","size":2},"layers":[{"mediaType":"%s","digest":"%s","size":%d}]}`,
		configDigest, mediaType, layerDigest, len(payload))
	return func(req *http.Request) (*http.Response, error) {
		onFetch()
		switch req.URL.Path {
		case "/v2/artifacts/manifests/" + ref:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(manifestPayload)),
				Header:     make(http.Header),
			}, nil
		case "/v2/artifacts/blobs/" + layerDigest:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(payload)),
				Header:     make(http.Header),
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}
	}
}

func artifactBlobDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestMatchCarriesWheelDigestAndAction(t *testing.T) {
	digest := artifact.WheelKey{SourceDigest: "sha256:abc", PyTag: "cp311", PlatformTag: "manylinux2014_s390x", RuntimeDigest: "rt"}.Digest()
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{{Name: "demo", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"}},
		DAG: []plan.DAGNode{
			{
				ID:       artifact.ID{Type: artifact.WheelType, Digest: digest},
				Type:     plan.NodeWheel,
				Action:   "reuse",
				Metadata: map[string]any{"name": "demo", "version": "1.0.0", "python_tag": "cp311", "platform_tag": "manylinux2014_s390x"},
				Inputs:   []artifact.ID{{Type: artifact.PackType, Digest: "sha256:pack1"}},
			},
		},
	}
	w := &Worker{Cfg: Config{}}
	reqs := []queue.Request{{Package: "demo", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"}}
	jobs := w.match(context.Background(), snap, reqs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].WheelDigest != digest {
		t.Fatalf("wheel digest not propagated: %s", jobs[0].WheelDigest)
	}
	if jobs[0].WheelAction != "reuse" {
		t.Fatalf("wheel action not propagated: %s", jobs[0].WheelAction)
	}
	if len(jobs[0].PackPaths) != 0 {
		t.Fatalf("pack paths should be empty without fetch")
	}
}

func TestCasURLHelper(t *testing.T) {
	w := &Worker{Cfg: Config{CASRegistryURL: "http://zot:5000", CASRegistryRepo: "artifacts"}}
	u := w.casURL(artifact.ID{Type: artifact.WheelType, Digest: "sha256:dead"})
	if u != "http://zot:5000/v2/artifacts/manifests/sha256-dead" {
		t.Fatalf("unexpected url: %s", u)
	}
}

type urlStore struct {
	url string
}

func (u urlStore) Put(_ context.Context, _ string, _ []byte, _ string) error { return nil }
func (u urlStore) Get(_ context.Context, _ string) ([]byte, string, error)   { return nil, "", io.EOF }
func (u urlStore) URL(_ string) string                                       { return u.url }

func TestObjectURLFallback(t *testing.T) {
	w := &Worker{Store: urlStore{url: "http://minio/bucket/path"}}
	u := w.objectURL(runner.Job{Name: "demo", Version: "1.0.0"}, "wheel")
	if u == "" {
		t.Fatalf("expected object url")
	}
}

func TestFetchRuntime(t *testing.T) {
	dir := t.TempDir()
	fetched := false
	tarBuf, rtDigest := sampleRuntimeTarWithDigest()
	w := &Worker{
		Cfg: Config{CacheDir: dir, LocalCASDir: filepath.Join(dir, "cas")},
		Fetcher: cas.Fetcher{
			BaseURL: "http://example",
			Repo:    "artifacts",
			Client: &http.Client{
				Transport: artifactFetchTransport(func() { fetched = true }, rtDigest, tarBuf.Bytes(), "application/octet-stream"),
			},
		},
		packPath: make(map[string]string),
	}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: rtDigest}
	path := w.fetchRuntime(context.Background(), "3.11", rtID, "reuse", nil, nil, nil)
	if path == "" {
		t.Fatalf("expected runtime path")
	}
	if !strings.HasSuffix(path, filepath.Join("usr", "local")) {
		t.Fatalf("expected prefix path, got %s", path)
	}
	if !fetched {
		t.Fatalf("fetcher not invoked for runtime")
	}
}

func TestFetchRuntimeUsesLocalCacheBeforeRemoteFetch(t *testing.T) {
	dir := t.TempDir()
	tarBuf, rtDigest := sampleRuntimeTarWithDigest()
	localCAS := filepath.Join(dir, "cas")
	tarPath := filepath.Join(localCAS, strings.ReplaceAll(rtDigest, ":", "_")+".tar")
	if err := os.MkdirAll(localCAS, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tarPath, tarBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &Worker{
		Cfg: Config{CacheDir: dir, LocalCASDir: localCAS},
		Fetcher: cas.Fetcher{
			BaseURL: "http://example",
			Repo:    "artifacts",
			Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatalf("remote fetch should not run when runtime tar is already cached locally")
				return nil, nil
			})},
		},
		packPath: make(map[string]string),
	}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: rtDigest}
	path := w.fetchRuntime(context.Background(), "3.11", rtID, "reuse", nil, nil, nil)
	if path == "" {
		t.Fatalf("expected cached runtime path")
	}
	if !strings.HasSuffix(path, filepath.Join("usr", "local")) {
		t.Fatalf("expected prefix path, got %s", path)
	}
}

func TestResolvePacksBuildsArtifacts(t *testing.T) {
	dir := t.TempDir()
	w := &Worker{
		Cfg: Config{
			CacheDir:       dir,
			LocalCASDir:    filepath.Join(dir, "cas"),
			DefaultPackCmd: samplePackBuildCmd(),
		},
		packPath: make(map[string]string),
	}
	packID := artifact.ID{Type: artifact.PackType, Digest: "sha256:packstub"}
	paths := w.resolvePacks(context.Background(), []artifact.ID{packID}, map[string]string{packID.Digest: "build"}, map[string]map[string]any{packID.Digest: {"name": "stub"}}, nil)
	if len(paths) != 1 || paths[0] == "" {
		t.Fatalf("expected built pack path")
	}
	if _, err := os.Stat(filepath.Join(paths[0], "include", "stub.h")); err != nil {
		t.Fatalf("expected built pack payload: %v", err)
	}
}

func TestResolvePacksUsesLocalCacheBeforeRemoteFetch(t *testing.T) {
	dir := t.TempDir()
	localCAS := filepath.Join(dir, "cas")
	if err := os.MkdirAll(localCAS, 0o755); err != nil {
		t.Fatal(err)
	}
	packID := artifact.ID{Type: artifact.PackType, Digest: "sha256:packstub"}
	tarPath := filepath.Join(localCAS, strings.ReplaceAll(packID.Digest, ":", "_")+".tar")
	if err := builder.BuildPack(tarPath, builder.PackBuildOpts{
		Digest:    packID.Digest,
		Meta:      map[string]any{"name": "stub"},
		Cmd:       samplePackBuildCmd(),
		LogWriter: io.Discard,
	}); err != nil {
		t.Fatalf("build cached pack artifact: %v", err)
	}
	w := &Worker{
		Cfg: Config{CacheDir: dir, LocalCASDir: localCAS},
		Fetcher: cas.Fetcher{
			BaseURL: "http://example",
			Repo:    "artifacts",
			Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatalf("remote fetch should not run when pack tar is already cached locally")
				return nil, nil
			})},
		},
		packPath: make(map[string]string),
	}
	paths := w.resolvePacks(context.Background(), []artifact.ID{packID}, map[string]string{packID.Digest: "reuse"}, map[string]map[string]any{packID.Digest: {"name": "stub"}}, nil)
	if len(paths) != 1 || paths[0] == "" {
		t.Fatalf("expected cached pack path")
	}
	if _, err := os.Stat(filepath.Join(paths[0], "include", "stub.h")); err != nil {
		t.Fatalf("expected cached pack payload: %v", err)
	}
}

func TestFetchRuntimeBuildsArtifacts(t *testing.T) {
	dir := t.TempDir()
	w := &Worker{Cfg: Config{
		CacheDir:          dir,
		LocalCASDir:       filepath.Join(dir, "cas"),
		DefaultRuntimeCmd: sampleRuntimeBuildCmd(),
	}}
	rtID := artifact.ID{Type: artifact.RuntimeType, Digest: "sha256:rt-stub"}
	path := w.fetchRuntime(context.Background(), "3.11", rtID, "build", map[string]any{"note": "stub"}, nil, nil)
	if path == "" {
		t.Fatalf("expected built runtime path")
	}
	if _, err := os.Stat(filepath.Join(path, "bin", "python3")); err != nil {
		t.Fatalf("expected runtime interpreter: %v", err)
	}
}

func TestRuntimeReadyUsesPrefixLibForProbe(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "usr", "local")
	binDir := filepath.Join(prefix, "bin")
	libDir := filepath.Join(prefix, "lib")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(binDir, "python3")
	payload := fmt.Sprintf("#!/bin/sh\ncase \":$LD_LIBRARY_PATH:\" in\n  *:%s:*) exit 0 ;;\n  *) exit 1 ;;\nesac\n", libDir)
	if err := os.WriteFile(script, []byte(payload), 0o755); err != nil {
		t.Fatal(err)
	}
	if !runtimeReady(prefix) {
		t.Fatalf("expected runtime probe to honor prefix lib dir")
	}
}

type stubQueue struct {
	reqs []queue.Request
}

func (s *stubQueue) Enqueue(ctx context.Context, req queue.Request) error { return nil }
func (s *stubQueue) List(ctx context.Context) ([]queue.Request, error)    { return nil, nil }
func (s *stubQueue) Clear(ctx context.Context) error                      { return nil }
func (s *stubQueue) Stats(ctx context.Context) (queue.Stats, error)       { return queue.Stats{}, nil }
func (s *stubQueue) Pop(ctx context.Context, max int) ([]queue.Request, error) {
	out := s.reqs
	s.reqs = nil
	return out, nil
}

type countingRunner struct {
	delay     time.Duration
	max       int32
	inFlight  int32
	totalRuns int32
}

func (r *countingRunner) Run(ctx context.Context, job runner.Job) (time.Duration, string, error) {
	cur := atomic.AddInt32(&r.inFlight, 1)
	defer atomic.AddInt32(&r.inFlight, -1)
	for {
		old := atomic.LoadInt32(&r.max)
		if cur <= old {
			break
		}
		if atomic.CompareAndSwapInt32(&r.max, old, cur) {
			break
		}
	}
	atomic.AddInt32(&r.totalRuns, 1)
	time.Sleep(r.delay)
	return r.delay, "", nil
}

func TestDrainRespectsBuildPoolSize(t *testing.T) {
	dir := t.TempDir()
	snap := plan.Snapshot{
		Plan: []plan.FlatNode{
			{Name: "a", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"},
			{Name: "b", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x", Action: "build"},
		},
	}
	if err := plan.Write(filepath.Join(dir, "plan.json"), snap); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	q := &stubQueue{reqs: []queue.Request{
		{Package: "a", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"},
		{Package: "b", Version: "1.0.0", PythonTag: "cp311", PlatformTag: "manylinux2014_s390x"},
	}}
	r := &countingRunner{delay: 50 * time.Millisecond}
	w := &Worker{
		Cfg: Config{
			OutputDir:     dir,
			CacheDir:      dir,
			BuildPoolSize: 1,
		},
		Queue:    q,
		Runner:   r,
		packPath: make(map[string]string),
	}
	if err := w.Drain(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if r.max > 1 {
		t.Fatalf("expected max concurrency 1, got %d", r.max)
	}
	if atomic.LoadInt32(&r.totalRuns) != 2 {
		t.Fatalf("expected 2 runs, got %d", r.totalRuns)
	}
}

func TestPopBuildQueueCapsMax(t *testing.T) {
	tests := []struct {
		name     string
		batch    int
		pool     int
		override int32
		wantMax  string
	}{
		{name: "pool overrides batch", batch: 10, pool: 4, wantMax: "4"},
		{name: "batch smaller than pool", batch: 2, pool: 5, wantMax: "2"},
		{name: "no batch uses pool", batch: 0, pool: 3, wantMax: "3"},
		{name: "override smaller than batch", batch: 10, pool: 5, override: 2, wantMax: "2"},
		{name: "no limits", batch: 0, pool: 0, wantMax: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMax := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMax = r.URL.Query().Get("max")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"builds":[]}`))
			}))
			defer srv.Close()

			worker := &Worker{
				Cfg: Config{
					ControlPlaneURL: srv.URL,
					BatchSize:       tt.batch,
					BuildPoolSize:   tt.pool,
				},
			}
			if tt.override > 0 {
				v := atomic.Int32{}
				v.Store(tt.override)
				worker.buildPoolSize = &v
			}

			if _, err := worker.popBuildQueue(context.Background()); err != nil {
				t.Fatalf("popBuildQueue: %v", err)
			}
			if gotMax != tt.wantMax {
				t.Fatalf("expected max=%q, got %q", tt.wantMax, gotMax)
			}
		})
	}
}

func sampleRuntimeTarWithDigest() (bytes.Buffer, string) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len("stub"))})
	_, _ = tw.Write([]byte("stub"))
	data := []byte("#!/bin/sh\nexit 0\n")
	_ = tw.WriteHeader(&tar.Header{Name: "usr/local/bin/python3", Mode: 0o755, Size: int64(len(data))})
	_, _ = tw.Write(data)
	_ = tw.Close()
	d := sha256.Sum256(buf.Bytes())
	return buf, "sha256:" + hex.EncodeToString(d[:])
}

func samplePackBuildCmd() string {
	return `mkdir -p "$PACK_OUTPUT/usr/local/include" && \
printf 'stub\n' > "$PACK_OUTPUT/usr/local/include/stub.h" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"stub","version":"1.0.0"}
EOF`
}

func sampleRuntimeBuildCmd() string {
	return `mkdir -p "$PACK_OUTPUT/usr/local/bin" && \
printf '#!/bin/sh\nexit 0\n' > "$PACK_OUTPUT/usr/local/bin/python3" && \
chmod +x "$PACK_OUTPUT/usr/local/bin/python3" && \
cat > "$PACK_OUTPUT/manifest.json" <<'EOF'
{"name":"cpython311","version":"3.11.0"}
EOF`
}
