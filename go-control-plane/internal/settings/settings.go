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
}

var mu sync.Mutex

const (
	defaultPythonVersion = "3.11"
	defaultPlatformTag   = "manylinux2014_s390x"
	defaultPollMs        = 10000
	defaultRecentLimit   = 25
)

// applyDefaults fills zero-values with sane defaults.
func applyDefaults(s Settings) Settings {
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
	return s
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
		return applyDefaults(Settings{})
	}
	var s Settings
	_ = json.Unmarshal(data, &s)
	return applyDefaults(s)
}

// Save writes settings to path, creating parent directories.
func Save(path string, s Settings) error {
	mu.Lock()
	defer mu.Unlock()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
