package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ArtifactType distinguishes artifact categories stored in CAS.
type ArtifactType string

const (
	RuntimeType ArtifactType = "runtime"
	PackType    ArtifactType = "pack"
	WheelType   ArtifactType = "wheel"
	RepairType  ArtifactType = "repair"
)

// ID is a typed reference to a content-addressed artifact.
type ID struct {
	Type   ArtifactType `json:"type"`
	Digest string       `json:"digest"`
}

// RuntimeKey describes a CPython runtime build.
type RuntimeKey struct {
	Arch             string `json:"arch"`
	PolicyBaseDigest string `json:"policy_base_digest"`
	PythonVersion    string `json:"python_version"`
	BuildFlags       string `json:"build_flags,omitempty"`
	ToolchainID      string `json:"toolchain_id,omitempty"`
	DepsHash         string `json:"deps_hash,omitempty"`
}

// PackKey describes a sysdep pack bundle.
type PackKey struct {
	Arch             string `json:"arch"`
	PolicyBaseDigest string `json:"policy_base_digest"`
	Name             string `json:"name"`
	Version          string `json:"version,omitempty"`
	RecipeDigest     string `json:"recipe_digest,omitempty"`
}

// WheelKey describes a built wheel artifact.
type WheelKey struct {
	SourceDigest          string   `json:"source_digest"`
	PyTag                 string   `json:"py_tag"`
	PlatformTag           string   `json:"platform_tag"`
	RuntimeDigest         string   `json:"runtime_digest"`
	PackDigests           []string `json:"pack_digests,omitempty"`
	BuildFrontendVersion  string   `json:"build_frontend_version,omitempty"`
	ConfigFlagsDigest     string   `json:"config_flags_digest,omitempty"`
	RepairToolVersion     string   `json:"repair_tool_version,omitempty"`
	RepairPolicyRulesHash string   `json:"repair_policy_rules_hash,omitempty"`
}

// RepairKey describes a post-build repair/compliance output.
type RepairKey struct {
	InputWheelDigest  string `json:"input_wheel_digest"`
	RepairToolVersion string `json:"repair_tool_version"`
	PolicyRulesDigest string `json:"policy_rules_digest"`
}

// Digest computes a stable content digest for the runtime key.
func (k RuntimeKey) Digest() string { return digestStruct(k) }

// Digest computes a stable content digest for the pack key.
func (k PackKey) Digest() string { return digestStruct(k) }

// Digest computes a stable content digest for the wheel key.
func (k WheelKey) Digest() string {
	sorted := k
	if len(sorted.PackDigests) > 1 {
		packs := append([]string(nil), sorted.PackDigests...)
		sort.Strings(packs)
		sorted.PackDigests = packs
	}
	return digestStruct(sorted)
}

// Digest computes a stable content digest for the repair key.
func (k RepairKey) Digest() string { return digestStruct(k) }

func digestStruct(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
