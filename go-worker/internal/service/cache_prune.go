package service

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type cacheEntry struct {
	path string
	size int64
	mod  time.Time
}

func (w *Worker) maybePruneCache() {
	limit := w.Cfg.CacheMaxBytes
	if limit <= 0 {
		return
	}
	interval := w.Cfg.CachePruneIntervalSec
	if interval <= 0 {
		interval = 300
	}
	now := time.Now().Unix()
	last := w.cachePruneAt.Load()
	if last > 0 && now-last < int64(interval) {
		return
	}
	w.cachePruneMu.Lock()
	defer w.cachePruneMu.Unlock()
	last = w.cachePruneAt.Load()
	if last > 0 && now-last < int64(interval) {
		return
	}
	w.cachePruneAt.Store(now)

	cacheDir := w.Cfg.CacheDir
	if cacheDir == "" {
		return
	}
	size, err := dirSize(cacheDir)
	if err != nil {
		log.Printf("cache prune: size error: %v", err)
		return
	}
	if size <= limit {
		return
	}
	need := size - limit
	freed, err := pruneCacheDirs(cacheDir, need)
	if err != nil {
		log.Printf("cache prune: %v", err)
		return
	}
	log.Printf("cache prune: freed %d bytes (target %d, total %d)", freed, need, size)
}

func pruneCacheDirs(cacheDir string, target int64) (int64, error) {
	var freed int64
	candidates := []string{"cas", "pip", "plans"}
	for _, name := range candidates {
		if freed >= target {
			break
		}
		path := filepath.Join(cacheDir, name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		delta, err := pruneOldest(path, target-freed)
		if err != nil {
			return freed, err
		}
		freed += delta
	}
	return freed, nil
}

func pruneOldest(dir string, target int64) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	candidates := make([]cacheEntry, 0, len(entries))
	for _, entry := range entries {
		full := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		size, err := entrySize(full, info)
		if err != nil {
			continue
		}
		candidates = append(candidates, cacheEntry{path: full, size: size, mod: info.ModTime()})
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].mod.Before(candidates[j].mod)
	})
	var freed int64
	for _, entry := range candidates {
		if freed >= target {
			break
		}
		if err := os.RemoveAll(entry.path); err != nil {
			return freed, err
		}
		freed += entry.size
	}
	return freed, nil
}

func entrySize(path string, info fs.FileInfo) (int64, error) {
	if !info.IsDir() {
		return info.Size(), nil
	}
	var size int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		stat, err := d.Info()
		if err != nil {
			return nil
		}
		size += stat.Size()
		return nil
	})
	return size, err
}

func dirSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return entrySize(path, info)
}
