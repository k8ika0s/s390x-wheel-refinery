package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// ManifestDigest returns a stable digest for immutable manifest enforcement.
// It hashes stable fields and sorted pack URLs; metadata is intentionally excluded
// to avoid nondeterministic map ordering in JSON payloads.
func ManifestDigest(entry ManifestEntry) (string, error) {
	payload := manifestDigestPayload{
		Name:        strings.ToLower(strings.TrimSpace(entry.Name)),
		Version:     strings.TrimSpace(entry.Version),
		Wheel:       strings.TrimSpace(entry.Wheel),
		WheelURL:    strings.TrimSpace(entry.WheelURL),
		RepairURL:   strings.TrimSpace(entry.RepairURL),
		RepairDigest: strings.TrimSpace(entry.RepairDigest),
		RuntimeURL:  strings.TrimSpace(entry.RuntimeURL),
		PythonTag:   strings.TrimSpace(entry.PythonTag),
		PlatformTag: strings.TrimSpace(entry.PlatformTag),
		Status:      strings.TrimSpace(entry.Status),
	}
	if len(entry.PackURLs) > 0 {
		payload.PackURLs = append([]string(nil), entry.PackURLs...)
		sort.Strings(payload.PackURLs)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type manifestDigestPayload struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Wheel        string   `json:"wheel"`
	WheelURL     string   `json:"wheel_url"`
	RepairURL    string   `json:"repair_url"`
	RepairDigest string   `json:"repair_digest"`
	RuntimeURL   string   `json:"runtime_url"`
	PackURLs     []string `json:"pack_urls,omitempty"`
	PythonTag    string   `json:"python_tag"`
	PlatformTag  string   `json:"platform_tag"`
	Status       string   `json:"status"`
}
