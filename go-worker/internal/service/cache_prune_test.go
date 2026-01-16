package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaybePruneCache(t *testing.T) {
	root := t.TempDir()
	casDir := filepath.Join(root, "cas")
	if err := os.MkdirAll(casDir, 0o755); err != nil {
		t.Fatalf("mkdir cas: %v", err)
	}
	oldPath := filepath.Join(casDir, "old.bin")
	newPath := filepath.Join(casDir, "new.bin")
	if err := os.WriteFile(oldPath, []byte("old-content"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new-content"), 0o644); err != nil {
		t.Fatalf("write new: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(oldPath, oldTime, oldTime)
	_ = os.Chtimes(newPath, newTime, newTime)

	w := &Worker{Cfg: Config{CacheDir: root, CacheMaxBytes: 15, CachePruneIntervalSec: 0}}
	w.maybePruneCache()

	entries, err := os.ReadDir(casDir)
	if err != nil {
		t.Fatalf("read cas: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(entries))
	}
	if entries[0].Name() != "new.bin" {
		t.Fatalf("expected newest entry to remain, got %s", entries[0].Name())
	}
}
