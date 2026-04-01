package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Settings are optional runtime-tunable knobs exposed to the UI.
type Settings struct {
	PythonVersion           string `json:"python_version,omitempty"`
	PlatformTag             string `json:"platform_tag,omitempty"`
	PollMs                  int    `json:"poll_ms,omitempty"`
	RecentLimit             int    `json:"recent_limit,omitempty"`
	AutoPlan                *bool  `json:"auto_plan,omitempty"`
	AutoBuild               *bool  `json:"auto_build,omitempty"`
	PlanPoolSize            int    `json:"plan_pool_size,omitempty"`
	BuildPoolSize           int    `json:"build_pool_size,omitempty"`
	InferEnabled            *bool  `json:"infer_enabled,omitempty"`
	InferModel              string `json:"infer_model,omitempty"`
	InferTimeoutSec         int    `json:"infer_timeout_sec,omitempty"`
	InferMaxRetries         int    `json:"infer_max_retries,omitempty"`
	InferSystemPrompt       string `json:"infer_system_prompt,omitempty"`
	InferUserPromptTemplate string `json:"infer_user_prompt_template,omitempty"`
}

var mu sync.Mutex
var (
	pythonVersionRe = regexp.MustCompile(`^3\.[0-9]{1,2}$`)
	platformTagRe   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

const (
	defaultPythonVersion           = "3.11"
	defaultPlatformTag             = "manylinux2014_s390x"
	defaultPollMs                  = 10000
	defaultRecentLimit             = 25
	defaultPlanPoolSize            = 2
	defaultBuildPoolSize           = 2
	defaultInferTimeout            = 20
	defaultInferRetries            = 1
	defaultInferSystemPrompt       = "You are a build-failure triage assistant. Return only a JSON object with: pattern, confidence (0-1), reason_code, summary, recipes (apt/dnf/pip/env arrays), notes, tags. Use minimal safe fixes."
	defaultInferUserPromptTemplate = "Package: {{package}}\nVersion: {{version}}\nPython: {{python}}\nPlatform: {{platform}}\nExisting recipes: {{existing_recipes}}\nLog excerpt:\n{{log_excerpt}}"
)

// ApplyDefaults fills zero-values with sane defaults, but preserves explicit false booleans.
func ApplyDefaults(s Settings) Settings {
	if s.PythonVersion == "" {
		s.PythonVersion = defaultPythonVersion
	}
	if s.PlatformTag == "" {
		s.PlatformTag = defaultPlatformTag
	}
	if s.PollMs == 0 {
		s.PollMs = defaultPollMs
	}
	if s.RecentLimit == 0 {
		s.RecentLimit = defaultRecentLimit
	}
	if s.PlanPoolSize == 0 {
		s.PlanPoolSize = defaultPlanPoolSize
	}
	if s.BuildPoolSize == 0 {
		s.BuildPoolSize = defaultBuildPoolSize
	}
	if s.InferTimeoutSec == 0 {
		s.InferTimeoutSec = defaultInferTimeout
	}
	if s.InferMaxRetries == 0 {
		s.InferMaxRetries = defaultInferRetries
	}
	if s.InferSystemPrompt == "" {
		s.InferSystemPrompt = defaultInferSystemPrompt
	}
	if s.InferUserPromptTemplate == "" {
		s.InferUserPromptTemplate = defaultInferUserPromptTemplate
	}
	// Auto modes default to false so queues require explicit enablement.
	if s.AutoPlan == nil {
		val := false
		s.AutoPlan = &val
	}
	if s.AutoBuild == nil {
		val := false
		s.AutoBuild = &val
	}
	if s.InferEnabled == nil {
		val := true
		s.InferEnabled = &val
	}
	return s
}

// Validate enforces basic sanity on user-supplied settings values.
func Validate(s Settings) error {
	py := s.PythonVersion
	if py != "" {
		if !pythonVersionRe.MatchString(py) {
			return fmt.Errorf("invalid python_version: %q (expected like 3.10)", py)
		}
	}
	pt := s.PlatformTag
	if pt != "" {
		if len(pt) > 64 || !platformTagRe.MatchString(pt) {
			return fmt.Errorf("invalid platform_tag: %q", pt)
		}
	}
	if s.InferTimeoutSec < 0 || s.InferTimeoutSec > 600 {
		return fmt.Errorf("invalid infer_timeout_sec: %d", s.InferTimeoutSec)
	}
	if s.InferMaxRetries < 0 || s.InferMaxRetries > 10 {
		return fmt.Errorf("invalid infer_max_retries: %d", s.InferMaxRetries)
	}
	if len(s.InferModel) > 256 {
		return fmt.Errorf("infer_model too long")
	}
	if len(s.InferSystemPrompt) > 32000 {
		return fmt.Errorf("infer_system_prompt too long")
	}
	if len(s.InferUserPromptTemplate) > 32000 {
		return fmt.Errorf("infer_user_prompt_template too long")
	}
	return nil
}

// BoolValue resolves a pointer bool to a concrete value (using false as the default).
func BoolValue(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// Load reads settings from path; returns zero Settings if file missing.
func Load(path string) Settings {
	mu.Lock()
	defer mu.Unlock()
	if path == "" {
		return Settings{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ApplyDefaults(Settings{})
	}
	var s Settings
	_ = json.Unmarshal(data, &s)
	return ApplyDefaults(s)
}

// Save writes settings to path, creating parent directories.
func Save(path string, s Settings) error {
	mu.Lock()
	defer mu.Unlock()
	if path == "" {
		return nil
	}
	s = ApplyDefaults(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
