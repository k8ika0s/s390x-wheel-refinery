package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SeedResult captures hint seeding stats.
type SeedResult struct {
	Files   int
	Loaded  int
	Skipped int
	Errors  []string
}

// SeedHintsFromDir loads YAML hint files from a directory and upserts them.
func SeedHintsFromDir(ctx context.Context, st Store, dir string) (SeedResult, error) {
	var res SeedResult
	if st == nil || dir == "" {
		return res, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	if !info.IsDir() {
		return res, fmt.Errorf("hints path is not a directory: %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		res.Files++
		data, err := os.ReadFile(path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		var hints []Hint
		if err := yaml.Unmarshal(data, &hints); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		for _, h := range hints {
			h = NormalizeHint(h)
			if errs := ValidateHint(h); len(errs) > 0 {
				label := h.ID
				if label == "" {
					label = "unknown-id"
				}
				res.Errors = append(res.Errors, fmt.Sprintf("%s:%s: %s", entry.Name(), label, strings.Join(errs, "; ")))
				res.Skipped++
				continue
			}
			if err := st.PutHint(ctx, h); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s:%s: %v", entry.Name(), h.ID, err))
				continue
			}
			res.Loaded++
		}
	}
	return res, nil
}
