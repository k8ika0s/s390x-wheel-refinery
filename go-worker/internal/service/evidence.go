package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
)

const autoFixPolicyVersion = "autofix-policy-2026-04-01-resilient-fallback"

var (
	stagePublishRe   = regexp.MustCompile(`(?i)(push|publish|manifest|zot|minio|uploadArtifacts|skip CAS push|digest mismatch)`)
	stageRepairRe    = regexp.MustCompile(`(?i)(auditwheel|repair\.sh|repair:|wheel repair)`)
	stageLinkRe      = regexp.MustCompile(`(?i)(undefined reference to|ld: cannot find|linker command failed|collect2: error|ld returned \d+ exit status)`)
	stageCompileRe   = regexp.MustCompile(`(?i)(gcc|g\+\+|clang|compilation terminated|error: subprocess-exited-with-error|meson|ninja|cmake error|cmake failed|fatal error:)`)
	stageDepsRe      = regexp.MustCompile(`(?i)(installing build dependencies|build dependencies|no module named|command not found|pkg-config|could not find|rust compiler not found|rustc: command not found)`)
	stageBootstrapRe = regexp.MustCompile(`(?i)(configure:|make(\[[0-9]+\])?:|cpython|runtime artifact|bootstrap:)`)
)

func promptVersion(cfg Config) string {
	systemPrompt := strings.TrimSpace(cfg.InferSystemPrompt)
	if systemPrompt == "" || systemPrompt == legacyInferenceSystemPrompt {
		systemPrompt = defaultInferenceSystemPrompt
	}
	userPrompt := strings.TrimSpace(cfg.InferUserPromptTemplate)
	if userPrompt == "" {
		userPrompt = defaultInferenceUserPromptTemplate
	}
	sum := sha256.Sum256([]byte(systemPrompt + "\n---\n" + userPrompt))
	return "prompt-" + hex.EncodeToString(sum[:6])
}

func effectiveEnvOverrides(recipes []string) []string {
	if len(recipes) == 0 {
		return nil
	}
	var out []string
	for _, recipe := range recipes {
		if strings.HasPrefix(recipe, "env:") {
			out = append(out, strings.TrimPrefix(recipe, "env:"))
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func failureStage(logContent string) string {
	logContent = strings.TrimSpace(logContent)
	if logContent == "" {
		return ""
	}
	tail := tailLogLines(logContent, 200)
	switch {
	case stagePublishRe.MatchString(tail):
		return "publish"
	case stageRepairRe.MatchString(tail):
		return "repair"
	case stageLinkRe.MatchString(tail):
		return "link"
	case stageCompileRe.MatchString(tail):
		return "compile"
	case stageDepsRe.MatchString(tail):
		return "dependency_install"
	case stageBootstrapRe.MatchString(tail):
		return "bootstrap"
	default:
		return "build"
	}
}

func failureExcerpt(logContent string) string {
	if strings.TrimSpace(logContent) == "" {
		return ""
	}
	lines := strings.Split(tailLogLines(logContent, 80), "\n")
	var selected []string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || isNoiseLine(line) {
			continue
		}
		selected = append(selected, line)
		if len(selected) == 8 {
			break
		}
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	excerpt := strings.Join(selected, "\n")
	if len(excerpt) > 1600 {
		excerpt = excerpt[len(excerpt)-1600:]
	}
	return strings.TrimSpace(excerpt)
}

func workerConfigMetadata(cfg Config) map[string]any {
	issues := []string{}
	if strings.TrimSpace(cfg.ControlPlaneURL) == "" {
		issues = append(issues, "control_plane_url_missing")
	}
	if cfg.BuildPoolSize <= 0 {
		issues = append(issues, "build_pool_invalid")
	}
	if cfg.PlanPoolSize <= 0 {
		issues = append(issues, "plan_pool_invalid")
	}
	if cfg.InferEnabled {
		if strings.TrimSpace(cfg.InferURL) == "" {
			issues = append(issues, "infer_url_missing")
		}
		if strings.TrimSpace(cfg.InferToken) == "" {
			issues = append(issues, "infer_token_missing")
		}
	}
	loaded := strings.EqualFold(strings.TrimSpace(os.Getenv("REFINERY_RUNTIME_ENV_LOADED")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("REFINERY_RUNTIME_ENV_LOADED")), "true")
	source := strings.TrimSpace(os.Getenv("REFINERY_RUNTIME_ENV_SOURCE"))
	if source == "" {
		source = "process-env"
	}
	meta := map[string]any{
		"config_ready":               len(issues) == 0,
		"config_drift":               len(issues) > 0,
		"config_issues":              issues,
		"infer_url_configured":       strings.TrimSpace(cfg.InferURL) != "",
		"infer_token_configured":     strings.TrimSpace(cfg.InferToken) != "",
		"infer_model":                strings.TrimSpace(cfg.InferModel),
		"prompt_version":             promptVersion(cfg),
		"policy_version":             autoFixPolicyVersion,
		"runtime_env_loaded":         loaded,
		"runtime_env_source":         source,
		"builder_profiles":           []string{builderProfileDefault, builderProfileNativeHeavy},
		"builder_image_default":      strings.TrimSpace(cfg.ContainerImage),
		"builder_image_native_heavy": firstNonEmpty(strings.TrimSpace(cfg.ContainerImageNativeHeavy), strings.TrimSpace(cfg.ContainerImage)),
	}
	return meta
}

func normalizeForMetadata(v any) any {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
