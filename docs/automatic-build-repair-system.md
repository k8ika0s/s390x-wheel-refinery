# Automatic Build and Repair System

This document explains the automated build and repair system in both plain language and technical terms. It includes a step by step walkthrough, how automation decisions are made, and where those decisions are surfaced in the UI.

## Purpose (Plain Language)
The system takes a list of Python packages, builds wheels in a controlled environment, and then tries to repair or adjust builds automatically when they fail. It keeps a full record of what it tried, what it changed, and why. Humans do not have to approve each step, but everything is visible and reviewable in the UI.

## Scope
- Build jobs are executed by workers in containers.
- Build failures can trigger automatic recipe application (installing extra system or Python dependencies, or setting environment variables).
- The system can infer new hints from logs and save them for reuse.
- Artifacts (wheels, repairs) are stored and tracked.
- All automation actions are recorded and shown in the UI.

## Glossary (Plain Terms)
- **Package**: The Python project being built.
- **Wheel**: The built binary package that gets installed by pip.
- **Plan**: A list of build tasks the system intends to run.
- **Queue**: A list of pending work items waiting for a worker.
- **Worker**: A service that pulls jobs from the queue and runs builds.
- **Runner**: The component that actually runs the build inside a container.
- **Hint**: A rule that says "if you see this error, try these fixes".
- **Recipe**: A small, explicit build change, like installing a package or setting an env var.
- **Auto-fix**: The logic that uses hints and log patterns to try fixes.
- **Attempt**: A numbered try for a specific build (1, 2, 3, ...).
- **Backoff**: A delay before retrying a failed build.
- **Repair**: A post-build step that produces a repaired wheel (for compliance or policy).

## Glossary (Technical)
- **Control-plane**: The API + database that stores plans, build status, events, hints, and logs.
- **Build queue**: The control-plane endpoint that leases build jobs to workers.
- **Plan queue**: The pipeline that generates build plans from inputs (requirements files).
- **Event**: A historical record of a build attempt (status + metadata).
- **Build status**: The current state of a build request (attempts, backoff, recipes, hint IDs).
- **Metadata**: Structured JSON payload attached to events.
- **CAS**: Content-addressed storage for immutable artifacts (wheels, repairs, packs).
- **Object store**: A blob storage backend for artifacts (S3 or similar).
- **Recipe format**: A string like `apt:libssl-dev`, `dnf:openssl-devel`, `pip:cryptography`, or `env:VAR=value`.
- **Hint applies_to**: Filters that constrain when a hint is valid (package, platform, python version, etc).

## System Overview (Plain Language)
1) A build job is queued.
2) A worker claims it and runs the build in a container.
3) If the build fails, the system checks known hints and log patterns.
4) If it finds a good match, it automatically applies recipes and retries.
5) If the logs suggest a new hint, it can be saved for future builds.
6) Every step, decision, and recipe is logged and shown in the UI.
7) If enabled, a repair pass creates a repaired wheel and stores it.
8) The build is done when it succeeds, hits max attempts, or is blocked by policy.

## Workflow Diagrams (Mermaid)
- High-level build workflow: `docs/diagrams/automatic-build-workflow.mmd`
- Auto-fix decision flow: `docs/diagrams/auto-fix-decision-flow.mmd`
- Repair pipeline: `docs/diagrams/repair-pipeline.mmd`
- Data + UX visibility: `docs/diagrams/automation-ux-visibility.mmd`
- Live log streaming: `docs/diagrams/log-streaming.mmd`

## End-to-End Walkthrough (Technical)
This walkthrough follows a single package through the automated build and repair pipeline.

### 1) Build request enters the queue
- A plan node (package + version + target tags) is queued for build.
- The control-plane stores a build_status row (package, version, attempts, recipes, hint IDs).

### 2) Worker leases work
- The worker calls the build queue endpoint and leases build jobs.
- Each leased job contains package, version, tags, attempts, and any pre-attached recipes or hint IDs.

### 3) Job execution
- The worker runs the job in a container using the runner.
- Job context is passed via env vars (`JOB_NAME`, `JOB_VERSION`, `PYTHON_TAG`, `PLATFORM_TAG`).
- If recipes are present, they are passed in `RECIPES` and executed in the container.

### 4) Build success path
- The worker emits a `built` event with duration and metadata.
- The wheel is stored to object storage and optionally to CAS.
- If repair is enabled, the worker produces a repaired wheel and stores it.
- The build_status row is updated to `built`.

### 5) Build failure path
- The worker emits a `failed` event with error metadata.
- The auto-fix engine inspects the last portion of the build log.
- Known hints are matched, and inferred hints are considered.
- If recipes are applied and attempts remain, the job is requeued as `retry`.
- If attempts are exhausted, the job ends as `failed`.

### 6) Retry and backoff
- Retries are scheduled with exponential backoff.
- Attempts are incremented by the control-plane when the job is leased.
- Backoff prevents immediate repeat failures and reduces thundering herd.

## Live Log Streaming (Worker → Control-Plane → UI)
Live logs are captured while the container runs and are visible in the UI without polling.

### How it works
1) The worker opens a streaming connection to the control-plane (`POST /api/logs/stream/{name}/{version}`).
2) As the runner emits stdout/stderr, the worker sends NDJSON log chunks over that stream.
3) The control-plane stores each chunk in the `log_chunks` table and broadcasts it to connected UI clients.
4) The UI pulls any existing chunks first (`GET /api/logs/chunks/{name}/{version}`), then keeps a WebSocket open to receive new chunks live.
5) When the job finishes, the worker still posts the final summarized log entry to `/api/logs` for long-term storage and export.

### Why we keep both
- **Chunks** give real-time tailing and preserve the raw stream.
- **Log entries** give a single summarized snapshot for quick downloads and history.

## Auto-Fix Engine (Detailed)
The auto-fix engine is the "intelligence" that turns logs into actionable changes.

### Inputs
- Build log content (last 200 lines are scanned by default).
- Job context (package, version, python version/tag, platform tag).
- Hint catalog from the control-plane.

### Matching known hints
1) Each hint has a regex `pattern`.
2) The pattern is matched against the log content.
3) If it matches, `applies_to` is checked against context (package, arch, platform, python).
4) Matching hints contribute recipes (apt/dnf/pip/env).

### Confidence gating
- Each hint includes `confidence` (low/medium/high or a float).
- The worker compares the hint confidence to `AUTO_FIX_MIN_CONFIDENCE`.
- Hints below the threshold are blocked and recorded in metadata.

### Log-based inference
If no known hint matches, the worker tries inference from common failure patterns, including:
- Missing Python modules (ModuleNotFoundError)
- Missing C headers (fatal error: header.h)
- Missing linker libraries (cannot find -lxxx)
- Missing pkg-config packages
- Missing CMake dependency (Could NOT find)
- Missing build tools (cmake, ninja, gcc, cargo)
- Missing Rust toolchain

When inference succeeds:
- A new hint is generated with a deterministic ID.
- It is only applied if confidence meets the threshold.
- The hint can be auto-saved (see below).

### Auto-save and dedupe
When `AUTO_SAVE_HINTS=true`:
- Inferred hints are saved to the control-plane catalog.
- Saves are rate-limited per package by `AUTO_HINT_RATE_LIMIT_MINUTES`.
- If a similar hint already exists, examples are merged instead of creating a duplicate.

### Recipe merging
- New recipes are merged with any existing recipes for the job.
- If the merged recipe set grows, auto-fix is considered "applied".
- The merged recipe list is recorded in event metadata and build status.

### Impact classification (safety signal)
- Recipes are classified as `normal` or `high` impact.
- High impact is flagged when recipes include large toolchains (gcc, clang, rust) or many dependencies.
- Impact is recorded in the automation metadata so humans can review.

## Recipe Execution Model
Recipes are serialized into the `RECIPES` environment variable and executed by the runner.

### Supported recipe prefixes
- `apt:<package>` - installed with `apt-get install`.
- `dnf:<package>` - installed with `dnf install`.
- `pip:<package>` - installed with `python -m pip install`.
- `env:KEY=VALUE` - exports environment variables inside the build.

### Example
```
RECIPES=apt:libssl-dev,dnf:openssl-devel,pip:cryptography,env:OPENSSL_NO_VENDOR=1
```

## Repair Pipeline
The repair flow produces a repaired wheel and publishes it when enabled.

### When repairs run
- Repair is enabled by `REPAIR_PUSH_ENABLED=true`.
- Repair attempts require a wheel digest and CAS/object store configured.

### Repair steps
1) A wheel is built and stored.
2) The worker looks for a repaired wheel output.
3) If missing, it runs the repair tool with configured policy/version.
4) The repaired artifact is verified and pushed to CAS/object storage.
5) Repair metadata (digest, tool version, policy hash) is recorded in the event.

## Data Stored and Tracked

### Build status (control-plane)
Fields include:
- package, version
- status (queued, building, retry, built, failed)
- attempts, backoff_until
- recipes (JSON list)
- hint_ids (string list)

### Events (control-plane)
Each build attempt writes an event with:
- status, detail, timestamp
- metadata (duration, attempts, artifacts)
- automation metadata (applied, recipes, hints, blocked hints, impact)

### Logs and manifests
- Logs are stored and accessible via the UI.
- Manifests track wheels, repairs, runtimes, and packs.

## UX Visibility (Where to Look)
The system is automated but visible at every step:
- **Events table** shows an automation summary per attempt.
- **Automation timeline** (package view) shows attempts, recipes, hint IDs, and blocks.
- **Event detail panel** shows full automation metadata and log links.
- **Build queue view** includes a recipes column for queued jobs.
- **Hints view** shows catalog entries and any auto-saved hints.

## Configuration
Key environment variables for automation:
- `AUTO_FIX_ENABLED` (default: true)
- `AUTO_SAVE_HINTS` (default: true)
- `AUTO_FIX_MIN_CONFIDENCE` (default: low)
- `AUTO_HINT_RATE_LIMIT_MINUTES` (default: 60)
- `REQUEUE_ON_FAILURE` (default: false)
- `MAX_REQUEUE_ATTEMPTS` (default: 3)
- `CONTROL_PLANE_URL` / `CONTROL_PLANE_TOKEN` (for hint access and status updates)
- `REPAIR_PUSH_ENABLED` (default: false)
- `REPAIR_TOOL_VERSION`, `REPAIR_POLICY_HASH`, `REPAIR_CMD` (repair settings)

## Safety and Guardrails
- Confidence gating prevents low-confidence hints from auto-applying.
- Rate limiting prevents noisy auto-saves for the same package.
- Impact classification flags potentially heavy recipe changes.
- Max attempts + exponential backoff prevent infinite retry loops.
- Full automation metadata is recorded for review.

## Troubleshooting and Failure Modes
- **No hints applied**: Check `AUTO_FIX_ENABLED` and the hint catalog; verify regex patterns match logs.
- **Hints blocked**: Check `AUTO_FIX_MIN_CONFIDENCE` and hint confidence values.
- **No auto-saved hints**: Check `AUTO_SAVE_HINTS` and rate limit window.
- **No retries**: Check `REQUEUE_ON_FAILURE` and `MAX_REQUEUE_ATTEMPTS`.
- **Repair missing**: Ensure `REPAIR_PUSH_ENABLED` and repair tool settings are configured.

## Reference: Files and Components
- Auto-fix logic: `go-worker/internal/service/auto_fix.go`
- Hint matching: `go-worker/internal/plan/hints.go`
- Runner recipe execution: `go-worker/internal/runner/runner.go`
- Worker pipeline: `go-worker/internal/service/worker.go`
- Control-plane build status APIs: `go-control-plane/internal/api/handlers.go`
- UI automation display: `ui/src/App.jsx`
