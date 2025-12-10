# s390x Wheel Refinery

Refine arbitrary Python wheels into a coherent, reproducible set of s390x wheels. Drop wheels from any platform into `/input`, get rebuilt s390x wheels plus a manifest, live logs, and a history database out of `/output`.

## What it does
- **Scan & plan**: Parse input wheels, reuse pure/compatible ones, and plan rebuilds for the rest (pinned, eager, or fallback-latest).
- **Resilient builds**: Adaptive build variants (ordered by past success), timeouts, exponential backoff, per-attempt logs, and hint-derived recipe retries.
- **Dynamic recovery**: Catalog-driven hints (dnf/apt) for missing libs/headers (AI/ML-friendly), auto-apply suggestions, and bounded dependency expansion to auto-build missing Python deps.
- **Isolation & control**: Run in containers (Rocky/Fedora/Ubuntu presets or custom images) with CPU/mem limits; cache/output bind-mounted to keep the host clean.
- **Concurrency & scheduling**: Parallel builds with warmed cache/venv; shortest-first scheduling using observed durations.
- **Observability**: Manifest with rich metadata, SQLite history of all events, logs on disk, SSE log streaming, JSON APIs, and a React SPA control panel (charts, catalog, alerts).

## Quick start
```bash
pip install -e .
refinery \
  --input /input \
  --output /output \
  --cache /cache \
  --python 3.11 \
  --platform-tag manylinux2014_s390x \
  --container-preset rocky \
  --jobs 2 \
  --auto-apply-suggestions
```
- Exit code is non-zero if any requirements are missing or builds fail.
- To reprocess failed packages queued from the web UI: `refinery worker --input /input --output /output --cache /cache --python 3.11`

## Key flags
- Paths: `--input`, `--output`, `--cache`
- Targets: `--python`, `--platform-tag`
- Strategy: `--upgrade-strategy {pinned,eager}`, `--fallback-latest`
- Index: `--index-url`, `--extra-index-url`, `--trusted-host`
- Config: `--config` (TOML/JSON), `--manifest`, `--history-db`
- Recipes: `--allow-system-recipes`, `--no-system-recipes`, `--dry-run-recipes`, `--auto-apply-suggestions`
- Attempts: `--max-attempts`, `--attempt-timeout`, `--attempt-backoff-base`, `--attempt-backoff-max`
- Concurrency: `--jobs`, `--schedule {shortest-first,fifo}`
- Containers: `--container-image`, `--container-preset {rocky,fedora,ubuntu}`, `--container-engine`, `--container-cpu`, `--container-memory`
- Resilience: `--skip-known-failures`
- Targeted builds: `--only name` or `--only name==version` to limit which jobs run (repeatable)
- Logging: `--verbose`

## Pipeline
1) **Scan** wheels and classify reuse vs rebuild.
2) **Plan** deps (pinned/eager/fallback-latest), skip known failures, and auto-plan missing Python deps (bounded).
3) **Build** jobs with adaptive variants, timeouts, backoff, logs, hints, and hint-based retry; cache outputs and copy to `/output`.
4) **Emit** manifest (per-entry metadata: variant, attempt, log, duration, hints) and record every event in history.

## Containers
- Builder bases: `containers/rocky/Dockerfile`, `containers/fedora/Dockerfile`, `containers/ubuntu/Dockerfile`
- Go control-plane: `containers/go-control-plane/Dockerfile` (Postgres/Redis/Kafka ready)
- React SPA UI: `containers/ui/Dockerfile`
- Make targets: `make build-rocky|build-fedora|build-ubuntu|build-web TAG=latest REGISTRY=local` (defaults to `podman`; you can set `ENGINE=docker` if preferred). Or build UI directly with `podman build -f containers/ui/Dockerfile .` (or `docker build` if you must).

## Web UI / API
- Start Go control-plane: build with `containers/go-control-plane/Dockerfile`, run with Postgres/Redis/Kafka (see compose below), API at `:8080`.
- React SPA: build/run `containers/ui/Dockerfile` (or `npm install && npm run dev` in `ui/` with `VITE_API_BASE=http://localhost:8080`).
- Features: recent events, top failures, top slow packages, variant history, hint catalog, queue length, worker trigger, and retry enqueue. Logs stream via `/api/logs` (plus `/api/logs/stream/{name}/{version}` SSE). APIs for stats remain under `/api/*`.
- Retry queue: POST `/package/{name}/retry` (SPA button or API) stores a request in `<cache>/retry_queue.json` with suggested recipes. Run the worker to consume the queue and rebuild those packages.
- Queue visibility & trigger: SPA shows queue depth and offers “Run worker now,” which calls `/api/worker/trigger` (enabled when the control-plane has access to `/input`, `/output`, `/cache` or a worker webhook is configured).
- Auth: optional `WORKER_TOKEN` env protects queue/worker endpoints (supply via `X-Worker-Token` or `?token=` when invoking). Control-plane also honors `WORKER_TOKEN` for trigger/smoke and ingest endpoints.
- Queue CLI: `refinery queue --cache /cache` prints queue length; `--queue-path` overrides the default.
- Token cookie helper: `POST /api/session/token?token=<WORKER_TOKEN>` sets a `worker_token` cookie for the browser, avoiding query/header injection in the UI.

## Compose
- Go control-plane stack (Postgres/Redis/Kafka + control-plane + Go worker + UI): `podman compose -f docker-compose.control-plane.yml up` (API: :8080, UI: :3000). `docker compose` also works if you prefer Docker. Queue backend selectable via `QUEUE_BACKEND=file|redis|kafka` (compose defaults to redis). Kafka does not support queue clear; use file/redis for dev resets. Worker posts plan/manifest/logs to the Go control-plane when `CONTROL_PLANE_URL`/`CONTROL_PLANE_TOKEN` are set (compose wires these).
- Podman is expected inside the worker container host; leave `PODMAN_BIN` empty to stub for local smoke, set `PODMAN_BIN=podman` (or similar) for real builds. Override the build entrypoint with `WORKER_RUN_CMD` when needed.

## Manifest & history
- Manifest: `<output>/manifest.json` with status, path, detail, and metadata (variant, attempt, log_path, duration, hints).
- History DB: every reuse/build/fail/missing/system_recipe/attempt with metadata for queries, UI, and adaptive scheduling.

## Resilience details
- Catalog-driven hints (dnf/apt) including AI/ML libraries (OpenBLAS/LAPACK/torch/numpy headers, etc.).
- Auto-apply suggestions adds system_packages + recipe steps; one extra hint-based retry runs when hints exist.
- Bounded dependency expansion auto-plans missing Python deps (depth-limited).
- Adaptive variant ordering uses past success rates per package; exponential backoff between attempts; timeouts per attempt.
- Parents are requeued after dependency builds (bounded by depth/attempt budgets).

## Retry queue & worker
- Web UI/API enqueue retries into `<cache>/retry_queue.json` (includes requested version, python tag, platform tag, and recipe steps).
- Go worker service (container) drains the queue using Podman with `/input`, `/output`, `/cache` bind mounts. Default command inside the build container is `refinery --input /input --output /output --cache /cache --python $PYTHON_TAG --platform-tag $PLATFORM_TAG --only $JOB_NAME==$JOB_VERSION --jobs 1`; override with `WORKER_RUN_CMD`. Environment like `JOB_NAME/JOB_VERSION/PYTHON_TAG/PLATFORM_TAG/RECIPES` are injected.
- Queue backends: file/JSON, Redis, Kafka (configure via `QUEUE_BACKEND`, `REDIS_URL`, `KAFKA_BROKERS`).
- Worker endpoints: `/health`, `/ready`, and `POST /trigger` (optionally gated by `WORKER_TOKEN`). Control-plane can call the worker webhook; the UI “Run worker now” button calls the control-plane which forwards to the worker.
- Useful for follow-up retries after triage without re-running the full pipeline; the queue is drained in batches.
- Podman expectations: set `PODMAN_BIN=podman` (or a stub for local dry-runs). Override the container-side build command with `WORKER_RUN_CMD` when needed; default is `refinery --input /input --output /output --cache /cache --python $PYTHON_TAG --platform-tag $PLATFORM_TAG --only $JOB_NAME==$JOB_VERSION --jobs 1`.
- Optional requeue on failure: set `REQUEUE_ON_FAILURE=true` and `MAX_REQUEUE_ATTEMPTS` to automatically push failed jobs back onto the queue with attempt counts.

## Scheduling & resources
- Shortest-first uses recorded avg durations; FIFO available.
- Container CPU/mem flags restrict build containers.
- Warm venv/cache once before high `--jobs` for best concurrency.

## Data locations
- Output wheels: `/output`
- Cache (sdists/wheels/logs): `/cache`
- Logs: `/cache/logs/<pkg>-<ver>-attemptN-<variant>.log`
- Manifest: `/output/manifest.json`
- History DB: `/cache/history.db`

## Testing
- Install dev deps: `pip install -e .[dev]`
- Run tests: `pytest -q`
- CI: GitHub Actions runs ruff lint, pytest, a web image build smoke, and a dummy-wheel orchestration smoke run on pushes/PRs, with pip caching for speed. Optional manual s390x emulated build is available via workflow dispatch.
