# s390x Wheel Refinery User Guide

This guide is written in plain language for day to day use. It explains how to upload inputs, generate plans, run builds, and read logs from the UI.

## What this system does
- Takes Python package lists or wheel files you upload.
- Builds missing s390x wheels in a controlled container.
- Retries failed builds with automatic fixes when possible.
- Streams build logs live in the UI.
- Stores outputs (wheels, repairs, manifests) in object storage and/or CAS, with local staging for debugging.
- Records structured remediation evidence for each attempt so you can later see what failed, what changed, and what drove the retry.

If you are new, think of it as a factory: you drop in a shopping list (requirements), and the system produces s390x wheels, while recording every step and retry it makes.

## Quick start (local)
1) Start the stack:
```
podman compose -f podman-compose.yml up
```
2) Open the UI:
```
http://localhost:3000
```
3) Upload a `requirements.txt` in the Inputs tab.
4) Watch the plan and build progress in Plans and Builds.

## Quick start (zkd0)
Use `zkd0` for real runtime validation because the service images are built for
`linux/s390x`.

1) Sync the repo to `~/s390x-wheel-refinery`.
2) Build images with host networking:
```
./scripts/build-stack-images-hostnet.sh
./scripts/publish-builder-image.sh
```
3) Start the host-network forge stack:
```
COMPOSE_FILE=podman-compose.hostnet.yml ./scripts/stack-up-no-build.sh
```
4) Open the control-plane at `http://127.0.0.1:18080` and the UI through its
host-network port/proxy for that deployment.

If optional monitoring image pulls are flaky, use the forge-only stack:
```
COMPOSE_FILE=podman-compose.core.yml ./scripts/stack-up-no-build.sh
```

## Core concepts in simple terms
- **Pending input**: An uploaded requirements file or wheel waiting to be planned.
- **Plan**: A list of build jobs (the DAG) created from your input.
- **Build job**: A single package/version that needs a wheel.
- **Worker**: The service that runs builds.
- **Auto plan**: Uploads are automatically sent to planning.
- **Auto build**: Plans automatically enqueue builds.
- **Hints and recipes**: Known fixes for build failures (install a system lib, set env var, etc).

## Typical flow (UI)
1) Upload a `requirements.txt` (Inputs page).
2) The file appears in Pending Inputs.
3) If auto plan is on, planning starts automatically. If not, click Enqueue.
4) A plan is saved and appears in the Plans page.
5) If auto build is on, builds are enqueued automatically. If not, click Enqueue builds.
6) The Builds page shows the queue and status.
7) Click a package to see events and live logs.

## Uploading inputs
### Requirements file (recommended)
Your file can be as simple as:
```
six==1.16.0
requests==2.32.3
```

Upload from the UI:
- Inputs page -> Upload requirements.txt

Or upload with the API:
```
curl -X POST -F "file=@requirements.txt" http://localhost:8080/api/requirements/upload
```

Example input sets for first validation live in:
- `examples/requirements/easy-common.txt`
- `examples/requirements/mixed-common-native.txt`

### Wheel file
Upload a wheel if you already have one and want it indexed:
```
curl -X POST -F "file=@package.whl" http://localhost:8080/api/wheels/upload
```

## Planning
Planning turns your input into a build graph.

### Auto plan
- If `AUTO_PLAN=true` on the control-plane, new uploads go straight into planning.
- The pending input will move from `pending` to `planning`.

### Manual plan
- Inputs page -> Enqueue (or Replan on failed).
- API:
```
POST /api/pending-inputs/{id}/enqueue-plan
```

## Building
Builds are stored in `build_status` and drained by the worker.

### Auto build (control-plane + worker)
- If the control-plane has `AUTO_BUILD=true`, plans enqueue builds right away.
- If the worker has `AUTO_BUILD=true`, it continuously polls the build queue.
- Both must be on for fully automatic builds.

### Manual build
- Plans page -> Enqueue builds
- Per-node enqueue buttons exist for individual build nodes.
- API:
```
POST /api/plans/{id}/enqueue-builds
POST /api/plans/{id}/enqueue-build
```

## Watching progress
### Builds page
- Shows queue status and active jobs.
- Expand a row to see timestamps, plan/run IDs, recipes, and errors.
- Build rows now keep `leased` distinct from `building`, so long runtime/pack
  bootstrap work is visible separately from queue ownership.

### Package view
- Shows event history and automation timeline.
- Log viewer includes live tail, search, highlight, wrap, and download.
- Recent attempts now include richer remediation evidence such as failure stage,
  failure excerpt, remediation source, prompt/policy version, and effective
  recipe/env changes when available.

### Log streaming
- Live logs stream into the UI while a build runs.
- Final logs are also stored for download.

## Auto fixes, hints, and recipes
When a build fails, the worker:
1) Checks the hint catalog for known fixes.
2) Applies deterministic heuristics for obvious classes such as missing tools,
   compiler mismatches, or missing system dependencies.
3) Falls back to external inference when configured and the failure is still
   ambiguous.
4) Normalizes any accepted suggestion into concrete `apt`, `dnf`, `pip`, and
   `env` recipes.
5) Retries automatically if attempts remain.

You can review:
- Applied recipes and hints in events.
- Auto-saved hints in the Hints page.
- Decision traces, remediation source, and ignored/blocked suggestions in event
  metadata and attempt history.

## Seed build for quick testing
Use the seed script to enqueue a known simple package and watch logs:
```
API_BASE=http://localhost:8080 PACKAGE=six VERSION=1.16.0 ./scripts/seed-build.sh
```
This will upload a tiny requirements file, create a plan, enqueue builds, and tail logs until completion.

## Status glossary
### Pending input
- `pending`: uploaded, waiting for plan enqueue
- `planning`: enqueued to plan queue
- `planned`: plan saved and linked
- `queued` or `build_queued`: builds enqueued
- `failed`: planning failed

### Build
- `pending`: waiting in build_status
- `leased`: claimed by worker
- `building`: container running
- `retry`: requeued after auto-fix
- `built`: succeeded
- `failed`: failed with no retry
- `cached` / `reused`: reused existing output
- `missing`: missing input
- `skipped_known_failure`: known failure skip
- `system_recipe_failed`: system recipe failed before build

## Troubleshooting
### Nothing is building
- Check auto build in Settings (control-plane).
- Check worker auto build (worker env `AUTO_BUILD=true`).
- Verify the worker is running and reporting heartbeats.
- On `zkd0`, check `/api/workers` or `/api/metrics` for `configured` workers and
  config drift before assuming the queue is broken.

### Input stuck in pending
- Auto plan may be off.
- Use Enqueue in the Inputs page.

### Build stuck in leased/building
- Worker may have died mid-run.
- Long runtime/bootstrap work now refreshes active builds while logs are still
  streaming, so a genuine stuck build is more likely to be a runner/runtime
  issue than a stale lease.

### No logs appear
- Check worker token in the worker environment (`WORKER_TOKEN`).
- Confirm the worker can reach `/api/logs/stream`.
- Try opening `/api/logs/chunks/{name}/{version}` directly.

### A worker is online but not ready
- Open the Workers view or call `/api/workers`.
- Check `config_ready`, `config_drift`, `infer_url_configured`, `infer_token_configured`, `prompt_version`, and `runtime_env_source`.
- If `runtime_env_source` is not your expected env file, restart with `./scripts/stack-up-no-build.sh` so runtime overrides are reloaded.

### Token issues
- Set `UI_TOKEN` for UI actions and `WORKER_TOKEN` for worker actions.
- The UI sends `X-UI-Token` and the worker sends `X-Worker-Token`.
- External inference uses worker-only `INFER_URL` / `INFER_TOKEN`; the prompt
  text and retry/model policy are managed through `/api/settings`.

## File locations
These paths are inside the worker container or bind mounts:
- `/cache/plans` - plan snapshots
- `/cache/cas` - CAS staging
- `/cache/pip` - pip downloads
- `/output` - local staging for built wheels and manifests (not the long-term source of truth)

Shared storage:
- **MinIO/S3** for uploaded inputs and published artifacts when enabled.
- **Zot/CAS** for immutable artifacts (wheels, runtimes, packs) when enabled.

## Artifact library and future pip repo
The long-term goal is to grow object storage into a large, searchable library of s390x wheels across Python versions, packages, and versions. Once it is hardened and populated, this library can be exposed as a pip-compatible repository for s390x builders.

## Where to go next
- `docs/automatic-build-repair-system.md` for deep technical detail.
- `docs/plan-build-queues.md` for the queue model.
- `docs/diagrams/README.md` for diagram index.
- `/api/metrics` and `/metrics` for readiness counters such as first-attempt success, retry success, hint application rate, and worker configuration drift.
