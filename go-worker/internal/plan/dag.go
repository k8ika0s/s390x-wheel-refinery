package plan

import "github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"

// NodeType distinguishes artifact stages in the DAG.
type NodeType string

const (
	NodeRuntime NodeType = "runtime"
	NodePack    NodeType = "pack"
	NodeWheel   NodeType = "wheel"
	NodeRepair  NodeType = "repair"
)

// DAGNode represents a content-addressed artifact and its dependencies.
type DAGNode struct {
	ID       artifact.ID    `json:"id"`
	Type     NodeType       `json:"type"`
	Inputs   []artifact.ID  `json:"inputs,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Action   string         `json:"action,omitempty"` // build or reuse
}

// DAG is a planner output: nodes plus optional metadata.
type DAG struct {
	RunID  string         `json:"run_id"`
	Nodes  []DAGNode      `json:"nodes"`
	Extras map[string]any `json:"extras,omitempty"`
}

// BuildRequest describes desired outputs and policies for planning.
type BuildRequest struct {
	Targets     []Target          `json:"targets"`
	Policy      string            `json:"policy,omitempty"`
	Strategy    string            `json:"strategy,omitempty"` // pinned/eager
	Constraints Constraints       `json:"constraints,omitempty"`
	Overrides   map[string]string `json:"overrides,omitempty"`
}

// Target is a single package + python combination (or requirement ref).
type Target struct {
	SourceRef      string   `json:"source_ref"`                // package/version or sdist URL
	PythonVersions []string `json:"python_versions,omitempty"` // e.g., ["3.11"]
	PlatformTag    string   `json:"platform_tag,omitempty"`    // e.g., manylinux2014_s390x
}

// Constraints describe planner knobs.
type Constraints struct {
	MaxDeps           int      `json:"max_deps,omitempty"`
	PacksAllowed      []string `json:"packs_allowed,omitempty"`
	PacksDenied       []string `json:"packs_denied,omitempty"`
	AllowRuntimeBuild bool     `json:"allow_runtime_build,omitempty"`
}
