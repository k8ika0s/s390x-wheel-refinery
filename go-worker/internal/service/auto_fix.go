package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

type autoFixResult struct {
	Applied              bool
	Recipes              []string
	ProposedRecipes      []string
	HintIDs              []string
	SavedHintIDs         []string
	Reason               string
	Source               string
	BlockedReason        string
	BlockedHints         []string
	Impact               string
	ImpactReason         string
	DecisionTrace        []string
	RawLLMOutput         string
	NormalizedLLMOutput  any
	PromptVersion        string
	LLMSuggestionIgnored bool
	LLMIgnoreReason      string
	HintSaveFailed       bool
}

type autoFixState struct {
	lastApplied   time.Time
	lastSignature string
}

const hintSaveTimeout = 5 * time.Second

func (w *Worker) autoFix(ctx context.Context, job runner.Job, logContent string, hints []plan.Hint, knownHints map[string]bool) autoFixResult {
	if !w.Cfg.AutoFixEnabled {
		return autoFixResult{DecisionTrace: []string{"auto-fix disabled"}}
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
	threshold := autoFixThreshold(w.Cfg.AutoFixMinConfidence)
	trace := []string{}
	addTrace := func(format string, args ...any) {
		trace = append(trace, fmt.Sprintf(format, args...))
	}

	var matchedIDs []string
	var blocked []string
	var recipes []string
	var saved []string
	var reason string
	var remediationSource string
	var hintSaveFailed bool
	var rawLLMOutput string
	var normalizedLLMOutput any
	var llmSuggestionIgnored bool
	var llmIgnoreReason string
	var currentPromptVersion = promptVersion(w.Cfg)
	applyInferred := func(hint plan.Hint, hintRecipes []string, note string, sourceLabel string) (bool, *autoFixResult) {
		addTrace("%s inferred hint pattern %s", sourceLabel, hint.Pattern)
		if hint.ID == "" {
			hint.ID = autoHintID(hint, ctxHint)
		}
		if confidenceScore(hint.Confidence) < threshold {
			addTrace("blocked inferred hint %s (confidence %s < %.2f)", hint.ID, hint.Confidence, threshold)
			log.Printf("auto-fix: %s@%s skip inferred hint confidence=%s", job.Name, job.Version, hint.Confidence)
			blocked = append(blocked, hint.ID)
			return true, &autoFixResult{
				BlockedReason: "confidence below threshold",
				BlockedHints:  dedupeStrings(blocked),
				DecisionTrace: trace,
			}
		}
		if existing, merged, ok := findSimilarHint(hints, hint); ok {
			hint = existing
			if merged {
				addTrace("merged inferred hint into %s", hint.ID)
				if w.Cfg.AutoSaveHints && w.Cfg.ControlPlaneURL != "" {
					if err := w.saveAutoHint(hint); err != nil {
						log.Printf("auto-fix: hint merge save failed for %s: %v", hint.ID, err)
					} else {
						saved = append(saved, hint.ID)
					}
				}
			}
		}
		if hint.ID == "" {
			hint.ID = autoHintID(hint, ctxHint)
		}
		if w.Cfg.AutoSaveHints && w.Cfg.ControlPlaneURL != "" {
			if w.canSaveAutoHint(job.Name) && (knownHints == nil || !knownHints[hint.ID]) {
				if err := w.saveAutoHint(hint); err != nil {
					log.Printf("auto-fix: hint save failed for %s: %v", hint.ID, err)
					addTrace("hint save failed: %s", hint.ID)
					hintSaveFailed = true
				} else {
					knownHints[hint.ID] = true
					w.markAutoHintSaved(job.Name)
					saved = append(saved, hint.ID)
					addTrace("saved inferred hint %s", hint.ID)
				}
			} else if !w.canSaveAutoHint(job.Name) {
				log.Printf("auto-fix: rate limit hit for %s; hint not saved", job.Name)
				if reason == "" {
					reason = "rate limit: hint not saved"
				}
				addTrace("rate limit hit: hint not saved")
			}
		}
		matchedIDs = append(matchedIDs, hint.ID)
		recipes = append(recipes, hintRecipes...)
		if reason == "" {
			reason = note
		}
		if autoSource := strings.TrimSpace(sourceLabel); autoSource != "" {
			remediationSource = autoSource
		}
		if len(hintRecipes) > 0 {
			addTrace("inferred recipes: %s", strings.Join(hintRecipes, ", "))
		}
		return false, nil
	}
	for _, h := range hints {
		_, recs, ok := plan.MatchHintForLog(h, ctxHint, logScan)
		if !ok {
			continue
		}
		score := confidenceScore(h.Confidence)
		if score < threshold {
			blocked = append(blocked, h.ID)
			addTrace("blocked hint %s (confidence %.2f < %.2f)", h.ID, score, threshold)
			autoFixBlocked := fmt.Sprintf("%s (confidence %.2f)", h.ID, score)
			log.Printf("auto-fix: %s@%s skip hint %s", job.Name, job.Version, autoFixBlocked)
			continue
		}
		addTrace("matched hint %s (confidence %.2f)", h.ID, score)
		if remediationSource == "" {
			remediationSource = "known_hint"
		}
		matchedIDs = append(matchedIDs, h.ID)
		for _, r := range recs {
			if r.Name != "" {
				recipes = append(recipes, r.Name)
			}
		}
	}

	if len(recipes) == 0 {
		if hint, hintRecipes, note, ok := inferHintFromLog(logScan, ctxHint); ok {
			if done, result := applyInferred(hint, hintRecipes, note, "heuristic"); done {
				return *result
			}
		}
	}
	if len(recipes) == 0 {
		decision := w.inferHintFromLLMDecision(ctx, logScan, ctxHint, job.Recipes)
		if len(decision.Trace) > 0 {
			trace = append(trace, decision.Trace...)
		}
		rawLLMOutput = decision.RawOutput
		normalizedLLMOutput = decision.NormalizedOutput
		if decision.PromptVersion != "" {
			currentPromptVersion = decision.PromptVersion
		}
		if decision.Ignored {
			llmSuggestionIgnored = true
			llmIgnoreReason = decision.IgnoreReason
		}
		if decision.OK {
			if done, result := applyInferred(decision.Hint, decision.Recipes, decision.Note, "llm"); done {
				return *result
			}
		}
	}

	merged := mergeRecipes(job.Recipes, recipes)
	applied := len(merged) > len(job.Recipes)
	signature := recipeSignature(merged)
	blockedReason := ""
	if applied && reason == "" {
		reason = "applied hint recipes"
	}
	if applied {
		if ok, guardReason := w.canApplyAutoFix(job, signature, merged, remediationSource); !ok {
			applied = false
			blockedReason = guardReason
		}
	}
	if applied {
		addTrace("applied recipes: %s", strings.Join(merged, ", "))
	} else if blockedReason != "" {
		addTrace("auto-fix blocked: %s", blockedReason)
	} else if len(recipes) == 0 {
		addTrace("no hint recipes matched")
	} else {
		addTrace("recipes already present; no change")
	}
	if applied {
		log.Printf("auto-fix: %s@%s applied recipes=%v hints=%v", job.Name, job.Version, merged, dedupeStrings(matchedIDs))
		w.markAutoFixApplied(job, signature)
	}
	impact, impactReason := recipeImpact(merged)
	if impact == "high" {
		if impactReason != "" {
			addTrace("high impact: %s", impactReason)
		} else {
			addTrace("high impact recipes detected")
		}
	}
	return autoFixResult{
		Applied:              applied,
		Recipes:              merged,
		ProposedRecipes:      dedupeStrings(recipes),
		HintIDs:              dedupeStrings(matchedIDs),
		SavedHintIDs:         dedupeStrings(saved),
		Reason:               reason,
		Source:               remediationSource,
		BlockedReason:        blockedReason,
		BlockedHints:         dedupeStrings(blocked),
		Impact:               impact,
		ImpactReason:         impactReason,
		DecisionTrace:        compactTrace(trace),
		RawLLMOutput:         rawLLMOutput,
		NormalizedLLMOutput:  normalizedLLMOutput,
		PromptVersion:        currentPromptVersion,
		LLMSuggestionIgnored: llmSuggestionIgnored,
		LLMIgnoreReason:      llmIgnoreReason,
		HintSaveFailed:       hintSaveFailed,
	}
}

func compactTrace(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func autoFixKey(job runner.Job) string {
	name := strings.TrimSpace(job.Name)
	version := strings.TrimSpace(job.Version)
	if name == "" || version == "" {
		return ""
	}
	return strings.ToLower(name) + "@" + strings.ToLower(version)
}

func recipeSignature(recipes []string) string {
	if len(recipes) == 0 {
		return ""
	}
	deduped := dedupeStrings(recipes)
	if len(deduped) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(deduped, "|"))
}

func (w *Worker) canApplyAutoFix(job runner.Job, signature string, recipes []string, source string) (bool, string) {
	if signature == "" {
		return true, ""
	}
	key := autoFixKey(job)
	if key == "" {
		return true, ""
	}
	w.autoFixMu.Lock()
	defer w.autoFixMu.Unlock()
	state, ok := w.autoFixState[key]
	if ok {
		if state.lastSignature != "" && state.lastSignature == signature {
			return false, "duplicate auto-fix already applied"
		}
		window := time.Duration(w.Cfg.AutoFixRateLimitMin) * time.Minute
		if window > 0 && !state.lastApplied.IsZero() && time.Since(state.lastApplied) < window && !canBypassAutoFixCooldown(recipes, source) {
			return false, "rate limit: auto-fix cooldown active"
		}
	}
	return true, ""
}

func canBypassAutoFixCooldown(recipes []string, source string) bool {
	if strings.TrimSpace(source) != "heuristic" {
		return false
	}
	if len(recipes) == 0 {
		return false
	}
	safe := map[string]bool{
		"findutils":   true,
		"cmake":       true,
		"ninja-build": true,
		"pkgconf":     true,
		"pkg-config":  true,
		"make":        true,
	}
	safeEnv := map[string]bool{
		"LD_LIBRARY_PATH=/opt/runtime/lib:/opt/runtime/lib64:${LD_LIBRARY_PATH:-}": true,
	}
	for _, recipe := range recipes {
		trimmed := strings.TrimSpace(recipe)
		if trimmed == "" {
			continue
		}
		var mgr, pkg string
		switch {
		case strings.HasPrefix(trimmed, "apt:"):
			mgr = "apt"
			pkg = strings.TrimSpace(strings.TrimPrefix(trimmed, "apt:"))
		case strings.HasPrefix(trimmed, "dnf:"):
			mgr = "dnf"
			pkg = strings.TrimSpace(strings.TrimPrefix(trimmed, "dnf:"))
		case strings.HasPrefix(trimmed, "env:"):
			envRecipe := strings.TrimSpace(strings.TrimPrefix(trimmed, "env:"))
			if !safeEnv[envRecipe] {
				return false
			}
			continue
		default:
			return false
		}
		if mgr == "" || pkg == "" || !safe[pkg] {
			return false
		}
	}
	return true
}

func (w *Worker) markAutoFixApplied(job runner.Job, signature string) {
	if signature == "" {
		return
	}
	key := autoFixKey(job)
	if key == "" {
		return
	}
	w.autoFixMu.Lock()
	w.autoFixState[key] = autoFixState{
		lastApplied:   time.Now(),
		lastSignature: signature,
	}
	w.autoFixMu.Unlock()
}

func (w *Worker) canSaveAutoHint(pkg string) bool {
	window := time.Duration(w.Cfg.AutoHintRateLimitMin) * time.Minute
	if window <= 0 || pkg == "" {
		return true
	}
	w.autoHintMu.Lock()
	defer w.autoHintMu.Unlock()
	last, ok := w.autoHintLast[strings.ToLower(pkg)]
	if !ok {
		return true
	}
	return time.Since(last) >= window
}

func (w *Worker) markAutoHintSaved(pkg string) {
	if pkg == "" {
		return
	}
	w.autoHintMu.Lock()
	w.autoHintLast[strings.ToLower(pkg)] = time.Now()
	w.autoHintMu.Unlock()
}

func autoFixThreshold(raw string) float64 {
	if raw == "" {
		return 0.0
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		return v
	}
	switch raw {
	case "low":
		return 0.3
	case "medium":
		return 0.6
	case "high":
		return 0.9
	default:
		return 0.0
	}
}

func confidenceScore(conf string) float64 {
	conf = strings.TrimSpace(strings.ToLower(conf))
	if conf == "" {
		return 0.3
	}
	if v, err := strconv.ParseFloat(conf, 64); err == nil {
		return v
	}
	switch conf {
	case "low":
		return 0.3
	case "medium":
		return 0.6
	case "high":
		return 0.9
	default:
		return 0.3
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
	if strings.Contains(strings.ToLower(logContent), "unable to get the locale encoding") &&
		strings.Contains(strings.ToLower(logContent), "no module named 'encodings'") {
		return plan.Hint{}, nil, "", false
	}

	gccTooOld := regexp.MustCompile(`(?i)requires gcc >=\s*([0-9.]+)`)
	if m := gccTooOld.FindStringSubmatch(logContent); len(m) == 2 {
		minVersion := strings.TrimSpace(m[1])
		hint := baseAutoHint(ctx, fmt.Sprintf(`requires GCC >= %s`, regexp.QuoteMeta(minVersion)))
		hint.Tags = append(hint.Tags, "compiler", "gcc")
		hint.Confidence = "high"
		hint.Note = fmt.Sprintf("Auto-detected compiler toolset requirement from build logs: GCC >= %s.", minVersion)
		hint.Recipes = map[string][]string{
			"dnf": {
				"gcc-toolset-12",
				"gcc-toolset-12-gcc",
				"gcc-toolset-12-gcc-c++",
			},
			"env": {
				"PATH=/opt/rh/gcc-toolset-12/root/usr/bin:$PATH",
				"CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc",
				"CXX=/opt/rh/gcc-toolset-12/root/usr/bin/g++",
				"LD_LIBRARY_PATH=/opt/rh/gcc-toolset-12/root/usr/lib64:${LD_LIBRARY_PATH:-}",
				"NPY_ALLOW_BLAS_UNSAFE=1",
			},
		}
		hint.Examples = []string{m[0]}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	libpythonMissing := regexp.MustCompile(`(?i)libpython3(?:\.[0-9]+)?\.so(?:\.[0-9.]+)?: cannot open shared object file`)
	if m := libpythonMissing.FindString(logContent); m != "" {
		hint := baseAutoHint(ctx, regexp.QuoteMeta(m))
		hint.Tags = append(hint.Tags, "runtime", "shared-library", "python")
		hint.Confidence = "high"
		hint.Note = "Auto-detected missing runtime libpython shared library path during Python startup. Re-prepend the mounted runtime library directories before running build steps."
		hint.Recipes = map[string][]string{
			"env": {
				"LD_LIBRARY_PATH=/opt/runtime/lib:/opt/runtime/lib64:${LD_LIBRARY_PATH:-}",
			},
		}
		hint.Examples = []string{m}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	missingModule := regexp.MustCompile(`ModuleNotFoundError: No module named ['"]([^'"]+)['"]`)
	if m := missingModule.FindStringSubmatch(logContent); len(m) == 2 {
		mod := strings.TrimSpace(m[1])
		if mod == "" {
			return plan.Hint{}, nil, "", false
		}
		hint := baseAutoHint(ctx, fmt.Sprintf(`ModuleNotFoundError: No module named ['"]%s['"]`, regexp.QuoteMeta(mod)))
		hint.Tags = append(hint.Tags, "missing-module", "python")
		hint.Confidence = "low"
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
		hint.Confidence = "medium"
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
		hint.Confidence = "medium"
		hint.Note = fmt.Sprintf("Auto-detected missing linker library %s from build logs.", lib)
		hint.Recipes = recipes
		hint.Examples = []string{m[0]}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	pkgConfigMissing := regexp.MustCompile(`No package '([^']+)' found|Package '([^']+)', required by 'virtual:world', not found`)
	if m := pkgConfigMissing.FindStringSubmatch(logContent); len(m) > 0 {
		name := ""
		for _, val := range m[1:] {
			if val != "" {
				name = val
				break
			}
		}
		if name != "" {
			recipes := libraryRecipes(name)
			hint := baseAutoHint(ctx, fmt.Sprintf(`No package '%s' found`, regexp.QuoteMeta(name)))
			hint.Tags = append(hint.Tags, "pkg-config", "missing-lib")
			hint.Confidence = "medium"
			hint.Note = fmt.Sprintf("Auto-detected missing pkg-config package %s from build logs.", name)
			hint.Recipes = recipes
			hint.Examples = []string{m[0]}
			return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
		}
	}

	cmakeMissing := regexp.MustCompile(`Could NOT find ([A-Za-z0-9_+.-]+)`)
	if m := cmakeMissing.FindStringSubmatch(logContent); len(m) == 2 {
		name := strings.TrimSpace(m[1])
		if name != "" {
			recipes := libraryRecipes(name)
			hint := baseAutoHint(ctx, fmt.Sprintf(`Could NOT find %s`, regexp.QuoteMeta(name)))
			hint.Tags = append(hint.Tags, "cmake", "missing-lib")
			hint.Confidence = "medium"
			hint.Note = fmt.Sprintf("Auto-detected missing CMake dependency %s from build logs.", name)
			hint.Recipes = recipes
			hint.Examples = []string{m[0]}
			return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
		}
	}

	missingTool := regexp.MustCompile(`(?:/bin/sh: )?([A-Za-z0-9_\-]+): command not found`)
	if m := missingTool.FindStringSubmatch(logContent); len(m) == 2 {
		tool := strings.TrimSpace(m[1])
		recipes := toolRecipes(tool)
		if len(recipes) > 0 {
			hint := baseAutoHint(ctx, fmt.Sprintf(`%s: command not found`, regexp.QuoteMeta(tool)))
			hint.Tags = append(hint.Tags, "missing-tool")
			hint.Confidence = "medium"
			hint.Note = fmt.Sprintf("Auto-detected missing tool %s from build logs.", tool)
			hint.Recipes = recipes
			hint.Examples = []string{m[0]}
			return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
		}
	}

	numpyGCCTooLow := regexp.MustCompile(`(?i)NumPy requires GCC\s*>=\s*([0-9.]+)`)
	if m := numpyGCCTooLow.FindStringSubmatch(logContent); len(m) == 2 {
		required := strings.TrimSpace(m[1])
		hint := baseAutoHint(ctx, fmt.Sprintf(`NumPy requires GCC >= %s`, regexp.QuoteMeta(required)))
		hint.Tags = append(hint.Tags, "compiler-version", "gcc", "numpy")
		hint.Confidence = "high"
		hint.Note = fmt.Sprintf("Auto-detected NumPy build dependency requiring GCC >= %s on a builder with an older compiler. Install a newer compiler toolset and force builds to use it.", required)
		hint.Recipes = map[string][]string{
			"dnf": {
				"gcc-toolset-12",
				"gcc-toolset-12-gcc",
				"gcc-toolset-12-gcc-c++",
			},
			"env": {
				"CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc",
				"CXX=/opt/rh/gcc-toolset-12/root/usr/bin/g++",
				"NPY_ALLOW_BLAS_UNSAFE=1",
			},
		}
		hint.Examples = []string{m[0]}
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
	}

	rustMissing := regexp.MustCompile(`(?i)rust compiler not found|rustc.*not found|cargo.*not found`)
	if m := rustMissing.FindStringSubmatch(logContent); len(m) > 0 {
		recipes := toolRecipes("cargo")
		if len(recipes) > 0 {
			hint := baseAutoHint(ctx, "rust compiler not found")
			hint.Tags = append(hint.Tags, "missing-tool", "rust")
			hint.Confidence = "medium"
			hint.Note = "Auto-detected missing Rust toolchain from build logs."
			hint.Recipes = recipes
			hint.Examples = []string{m[0]}
			return hint, flattenRecipeMap(hint.Recipes), hint.Note, true
		}
	}

	return plan.Hint{}, nil, "", false
}

func (w *Worker) saveAutoHint(hint plan.Hint) error {
	ctx, cancel := context.WithTimeout(context.Background(), hintSaveTimeout)
	defer cancel()
	return upsertHint(ctx, nil, w.Cfg, hint)
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
	case "xargs":
		return map[string][]string{"apt": {"findutils"}, "dnf": {"findutils"}}
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

func findSimilarHint(existing []plan.Hint, candidate plan.Hint) (plan.Hint, bool, bool) {
	candPattern := strings.TrimSpace(strings.ToLower(candidate.Pattern))
	candKey := hintRecipeKey(candidate)
	candPkg := hintPackage(candidate)
	for _, h := range existing {
		if candPattern != "" && strings.TrimSpace(strings.ToLower(h.Pattern)) == candPattern {
			merged, updated := mergeHintExamples(h, candidate.Examples)
			return merged, true, updated
		}
		if candKey != "" && hintRecipeKey(h) == candKey {
			if candPkg == "" || candPkg == hintPackage(h) {
				merged, updated := mergeHintExamples(h, candidate.Examples)
				return merged, true, updated
			}
		}
	}
	return plan.Hint{}, false, false
}

func hintRecipeKey(h plan.Hint) string {
	if len(h.Recipes) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(flattenRecipeMap(h.Recipes), "|"))
}

func hintPackage(h plan.Hint) string {
	if len(h.AppliesTo) == 0 {
		return ""
	}
	for k, vals := range h.AppliesTo {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "packages" || key == "package" || key == "package_names" {
			if len(vals) > 0 {
				return strings.ToLower(strings.TrimSpace(vals[0]))
			}
		}
	}
	return ""
}

func mergeHintExamples(h plan.Hint, examples []string) (plan.Hint, bool) {
	if len(examples) == 0 {
		return h, false
	}
	seen := make(map[string]bool)
	for _, ex := range h.Examples {
		seen[ex] = true
	}
	updated := false
	for _, ex := range examples {
		ex = strings.TrimSpace(ex)
		if ex == "" || seen[ex] {
			continue
		}
		seen[ex] = true
		h.Examples = append(h.Examples, ex)
		updated = true
	}
	return h, updated
}

func recipeImpact(recipes []string) (string, string) {
	if len(recipes) == 0 {
		return "", ""
	}
	if len(recipes) >= 6 {
		return "high", "bulk dependency install"
	}
	high := map[string]bool{
		"build-essential": true,
		"gcc":             true,
		"g++":             true,
		"clang":           true,
		"llvm":            true,
		"rust":            true,
		"rustc":           true,
		"cargo":           true,
		"gcc-c++":         true,
	}
	for _, recipe := range recipes {
		parts := strings.SplitN(recipe, ":", 2)
		if len(parts) == 0 {
			continue
		}
		mgr := strings.TrimSpace(parts[0])
		arg := ""
		if len(parts) > 1 {
			arg = strings.TrimSpace(parts[1])
		}
		if mgr == "env" {
			return "high", "environment override"
		}
		if mgr == "apt" || mgr == "dnf" || mgr == "pip" {
			for _, tok := range strings.Fields(arg) {
				if high[strings.ToLower(tok)] {
					return "high", fmt.Sprintf("installs %s", tok)
				}
			}
		}
	}
	return "normal", ""
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
