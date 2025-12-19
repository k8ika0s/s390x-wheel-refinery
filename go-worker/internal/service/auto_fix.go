package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

type autoFixResult struct {
	Applied       bool
	Recipes       []string
	HintIDs       []string
	SavedHintIDs  []string
	Reason        string
	BlockedReason string
}

func (w *Worker) autoFix(ctx context.Context, job runner.Job, logContent string, hints []plan.Hint, knownHints map[string]bool) autoFixResult {
	if !w.Cfg.AutoFixEnabled {
		return autoFixResult{}
	}
	if knownHints == nil {
		knownHints = map[string]bool{}
	}
	logScan := tailLogLines(logContent, 200)
	ctxHint := plan.HintContext{
		Package:       job.Name,
		Version:       job.Version,
		PythonVersion: job.PythonVersion,
		PythonTag:     job.PythonTag,
		PlatformTag:   job.PlatformTag,
	}

	var matchedIDs []string
	var recipes []string
	for _, h := range hints {
		match, recs, ok := plan.MatchHintForLog(h, ctxHint, logScan)
		if !ok {
			continue
		}
		matchedIDs = append(matchedIDs, h.ID)
		for _, r := range recs {
			if r.Name != "" {
				recipes = append(recipes, r.Name)
			}
		}
	}

	var saved []string
	var reason string
	if len(recipes) == 0 {
		if hint, hintRecipes, note, ok := inferHintFromLog(logScan, ctxHint); ok {
			if hint.ID == "" {
				hint.ID = autoHintID(hint, ctxHint)
			}
			if w.Cfg.AutoSaveHints && w.Cfg.ControlPlaneURL != "" {
				if knownHints == nil || !knownHints[hint.ID] {
					if err := upsertHint(ctx, nil, w.Cfg, hint); err != nil {
						log.Printf("auto-fix: hint save failed for %s: %v", hint.ID, err)
					} else {
						knownHints[hint.ID] = true
						saved = append(saved, hint.ID)
					}
				}
			}
			matchedIDs = append(matchedIDs, hint.ID)
			recipes = append(recipes, hintRecipes...)
			reason = note
		}
	}

	merged := mergeRecipes(job.Recipes, recipes)
	applied := len(merged) > len(job.Recipes)
	if applied && reason == "" {
		reason = "applied hint recipes"
	}
	if applied {
		log.Printf("auto-fix: %s@%s applied recipes=%v hints=%v", job.Name, job.Version, merged, dedupeStrings(matchedIDs))
	}
	return autoFixResult{
		Applied:      applied,
		Recipes:      merged,
		HintIDs:      dedupeStrings(matchedIDs),
		SavedHintIDs: dedupeStrings(saved),
		Reason:       reason,
	}
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, v := range in {
		t := strings.TrimSpace(v)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func tailLogLines(logContent string, maxLines int) string {
	if maxLines <= 0 {
		return logContent
	}
	lines := strings.Split(logContent, "\n")
	if len(lines) <= maxLines {
		return logContent
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func autoHintID(hint plan.Hint, ctx plan.HintContext) string {
	key := strings.Join([]string{
		strings.TrimSpace(hint.Pattern),
		strings.Join(hint.Tags, ","),
		strings.Join(hint.Recipes["apt"], ","),
		strings.Join(hint.Recipes["dnf"], ","),
		strings.Join(hint.Recipes["pip"], ","),
		ctx.Package,
		ctx.PlatformTag,
		ctx.PythonVersion,
	}, "|")
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("auto-%s", hex.EncodeToString(sum[:6]))
}

func inferHintFromLog(logContent string, ctx plan.HintContext) (plan.Hint, []string, string, bool) {
	missingModule := regexp.MustCompile(`ModuleNotFoundError: No module named ['"]([^'"]+)['"]`)
	if m := missingModule.FindStringSubmatch(logContent); len(m) == 2 {
		mod := strings.TrimSpace(m[1])
		if mod == "" {
			return plan.Hint{}, nil, "", false
		}
		hint := baseAutoHint(ctx, fmt.Sprintf(`ModuleNotFoundError: No module named ['"]%s['"]`, regexp.QuoteMeta(mod)))
		hint.Tags = append(hint.Tags, "missing-module", "python")
		hint.Note = fmt.Sprintf("Auto-detected missing Python module %s from build logs.", mod)
		hint.Recipes = map[string][]string{"pip": {mod}}
		hint.Examples = []string{m[0]}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	missingHeader := regexp.MustCompile(`fatal error: ([A-Za-z0-9_./\-]+\.h): No such file or directory`)
	if m := missingHeader.FindStringSubmatch(logContent); len(m) == 2 {
		header := strings.TrimSpace(m[1])
		base := headerBase(header)
		recipes := headerRecipes(base)
		hint := baseAutoHint(ctx, fmt.Sprintf(`fatal error: %s: No such file or directory`, regexp.QuoteMeta(header)))
		hint.Tags = append(hint.Tags, "missing-header")
		hint.Note = fmt.Sprintf("Auto-detected missing header %s from build logs.", header)
		hint.Recipes = recipes
		hint.Examples = []string{m[0]}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	missingLib := regexp.MustCompile(`cannot find -l([A-Za-z0-9_\-]+)`)
	if m := missingLib.FindStringSubmatch(logContent); len(m) == 2 {
		lib := strings.TrimSpace(m[1])
		recipes := libraryRecipes(lib)
		hint := baseAutoHint(ctx, fmt.Sprintf(`cannot find -l%s`, regexp.QuoteMeta(lib)))
		hint.Tags = append(hint.Tags, "missing-lib")
		hint.Note = fmt.Sprintf("Auto-detected missing linker library %s from build logs.", lib)
		hint.Recipes = recipes
		hint.Examples = []string{m[0]}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	missingTool := regexp.MustCompile(`(?:/bin/sh: )?([A-Za-z0-9_\-]+): command not found`)
	if m := missingTool.FindStringSubmatch(logContent); len(m) == 2 {
		tool := strings.TrimSpace(m[1])
		recipes := toolRecipes(tool)
		if len(recipes) > 0 {
			hint := baseAutoHint(ctx, fmt.Sprintf(`%s: command not found`, regexp.QuoteMeta(tool)))
			hint.Tags = append(hint.Tags, "missing-tool")
			hint.Note = fmt.Sprintf("Auto-detected missing tool %s from build logs.", tool)
			hint.Recipes = recipes
			hint.Examples = []string{m[0]}
			return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
		}
	}

	return plan.Hint{}, nil, "", false
}

func baseAutoHint(ctx plan.HintContext, pattern string) plan.Hint {
	applies := map[string][]string{}
	if ctx.Package != "" {
		applies["packages"] = []string{strings.ToLower(ctx.Package)}
	}
	if ctx.PlatformTag != "" {
		applies["platform_tags"] = []string{strings.ToLower(ctx.PlatformTag)}
	}
	if ctx.PythonVersion != "" {
		applies["python_versions"] = []string{ctx.PythonVersion}
	} else if ctx.PythonTag != "" {
		applies["python_tags"] = []string{strings.ToLower(ctx.PythonTag)}
	}
	if arch := archFromPlatformTag(ctx.PlatformTag); arch != "" {
		applies["arch"] = []string{arch}
	}
	if plat := platformFamily(ctx.PlatformTag); plat != "" {
		applies["platforms"] = []string{plat}
	}
	return plan.Hint{
		Pattern:    pattern,
		Tags:       []string{"auto", "generated"},
		Severity:   "error",
		Confidence: "low",
		AppliesTo:  applies,
	}
}

func flattenRecipeMap(recipes map[string][]string) []string {
	if len(recipes) == 0 {
		return nil
	}
	var out []string
	keys := make([]string, 0, len(recipes))
	for k := range recipes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, mgr := range keys {
		for _, step := range recipes[mgr] {
			trimmed := strings.TrimSpace(step)
			if trimmed == "" {
				continue
			}
			out = append(out, fmt.Sprintf("%s:%s", mgr, trimmed))
		}
	}
	return out
}

func headerBase(header string) string {
	base := header
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(base, ".h")
	base = strings.TrimPrefix(base, "lib")
	return strings.ToLower(base)
}

func headerRecipes(base string) map[string][]string {
	switch base {
	case "z", "zlib":
		return map[string][]string{"apt": {"zlib1g-dev"}, "dnf": {"zlib-devel"}}
	case "ssl", "openssl":
		return map[string][]string{"apt": {"libssl-dev"}, "dnf": {"openssl-devel"}}
	case "ffi":
		return map[string][]string{"apt": {"libffi-dev"}, "dnf": {"libffi-devel"}}
	case "bz2", "bzip2":
		return map[string][]string{"apt": {"libbz2-dev"}, "dnf": {"bzip2-devel"}}
	case "lzma", "xz":
		return map[string][]string{"apt": {"liblzma-dev"}, "dnf": {"xz-devel"}}
	case "png":
		return map[string][]string{"apt": {"libpng-dev"}, "dnf": {"libpng-devel"}}
	case "jpeg", "jpg":
		return map[string][]string{"apt": {"libjpeg-dev"}, "dnf": {"libjpeg-turbo-devel"}}
	case "xml2":
		return map[string][]string{"apt": {"libxml2-dev"}, "dnf": {"libxml2-devel"}}
	case "xslt":
		return map[string][]string{"apt": {"libxslt1-dev"}, "dnf": {"libxslt-devel"}}
	case "sqlite3", "sqlite":
		return map[string][]string{"apt": {"libsqlite3-dev"}, "dnf": {"sqlite-devel"}}
	default:
		if base == "" {
			return nil
		}
		return map[string][]string{
			"apt": {fmt.Sprintf("lib%s-dev", base)},
			"dnf": {fmt.Sprintf("%s-devel", base)},
		}
	}
}

func libraryRecipes(lib string) map[string][]string {
	if lib == "" {
		return nil
	}
	switch lib {
	case "z":
		return map[string][]string{"apt": {"zlib1g-dev"}, "dnf": {"zlib-devel"}}
	case "ssl", "crypto":
		return map[string][]string{"apt": {"libssl-dev"}, "dnf": {"openssl-devel"}}
	case "ffi":
		return map[string][]string{"apt": {"libffi-dev"}, "dnf": {"libffi-devel"}}
	case "bz2":
		return map[string][]string{"apt": {"libbz2-dev"}, "dnf": {"bzip2-devel"}}
	case "lzma":
		return map[string][]string{"apt": {"liblzma-dev"}, "dnf": {"xz-devel"}}
	case "png":
		return map[string][]string{"apt": {"libpng-dev"}, "dnf": {"libpng-devel"}}
	case "jpeg":
		return map[string][]string{"apt": {"libjpeg-dev"}, "dnf": {"libjpeg-turbo-devel"}}
	case "xml2":
		return map[string][]string{"apt": {"libxml2-dev"}, "dnf": {"libxml2-devel"}}
	case "xslt":
		return map[string][]string{"apt": {"libxslt1-dev"}, "dnf": {"libxslt-devel"}}
	case "sqlite3":
		return map[string][]string{"apt": {"libsqlite3-dev"}, "dnf": {"sqlite-devel"}}
	default:
		base := strings.TrimPrefix(strings.ToLower(lib), "lib")
		return map[string][]string{
			"apt": {fmt.Sprintf("lib%s-dev", base)},
			"dnf": {fmt.Sprintf("%s-devel", base)},
		}
	}
}

func toolRecipes(tool string) map[string][]string {
	switch strings.ToLower(tool) {
	case "cmake":
		return map[string][]string{"apt": {"cmake"}, "dnf": {"cmake"}}
	case "ninja":
		return map[string][]string{"apt": {"ninja-build"}, "dnf": {"ninja-build"}}
	case "pkg-config", "pkgconf":
		return map[string][]string{"apt": {"pkg-config"}, "dnf": {"pkgconf"}}
	case "rustc", "cargo":
		return map[string][]string{"apt": {"rustc", "cargo"}, "dnf": {"rust", "cargo"}}
	case "make":
		return map[string][]string{"apt": {"make"}, "dnf": {"make"}}
	case "gcc":
		return map[string][]string{"apt": {"gcc"}, "dnf": {"gcc"}}
	case "g++", "c++":
		return map[string][]string{"apt": {"g++"}, "dnf": {"gcc-c++"}}
	default:
		return nil
	}
}

func archFromPlatformTag(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.Split(strings.ToLower(tag), "_")
	return parts[len(parts)-1]
}

func platformFamily(tag string) string {
	tag = strings.ToLower(tag)
	switch {
	case strings.Contains(tag, "manylinux"), strings.Contains(tag, "linux"):
		return "linux"
	case strings.Contains(tag, "darwin"), strings.Contains(tag, "macos"):
		return "darwin"
	case strings.Contains(tag, "win"):
		return "windows"
	default:
		return ""
	}
}
