package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings are optional runtime-tunable knobs exposed to the UI.
type Settings struct {
	PythonVersion string `json:"python_version,omitempty"`
	PlatformTag   string `json:"platform_tag,omitempty"`
	PollMs        int    `json:"poll_ms,omitempty"`
	RecentLimit   int    `json:"recent_limit,omitempty"`
	AutoPlan      *bool  `json:"auto_plan,omitempty"`
	AutoBuild     *bool  `json:"auto_build,omitempty"`
}

var mu sync.Mutex

const (
	defaultPythonVersion = "3.11"
	defaultPlatformTag   = "manylinux2014_s390x"
	defaultPollMs        = 10000
	defaultRecentLimit   = 25
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
	// Auto modes default to true to match current behavior.
	if s.AutoPlan == nil {
		val := true
		s.AutoPlan = &val
	}
	if s.AutoBuild == nil {
		val := true
		s.AutoBuild = &val
	}
	return s
}

// BoolValue resolves a pointer bool to a concrete value (using true as the default).
func BoolValue(b *bool) bool {
	if b == nil {
		return true
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
