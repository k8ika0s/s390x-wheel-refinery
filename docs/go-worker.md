# Go Worker Service (Draft)

## Purpose (Layperson)
A fast, container-isolated worker that drains the retry/build queue and rebuilds wheels. It mounts `/output` and `/cache`, runs builds in a Podman container (Docker later), reads inputs from object storage metadata, and reports results (plan/manifest/logs/events) to the Go control-plane. This replaces the Python worker path entirely.

## Scope
- Queue backends: file/JSON, Redis, Kafka (same interface as control-plane). No dependency on Python queue.
- Builds: run via Podman with cache/output bind-mounts; presets (rocky/fedora/ubuntu) or custom image. Runner passes job context as env (`JOB_NAME`, `JOB_VERSION`, `PYTHON_TAG`, `PLATFORM_TAG`, optional `RECIPES`) and runs `WORKER_RUN_CMD` if provided; otherwise defaults to `refinery-build` (script in builder images) which invokes `refinery` inside the container using a scratch input dir. Podman-first; Docker is optional. Stubbed unless `PODMAN_BIN` is set.
- Plan generation: loads `plan.json` from /output or /cache; otherwise plans are provided via control-plane pending inputs (no `/input` directory needed). Resolver respects `INDEX_URL`, `EXTRA_INDEX_URL`, and `UPGRADE_STRATEGY` (`pinned` or `eager`).
- Reporting: streams live log chunks to the control-plane and posts manifest/logs/plan/events when `CONTROL_PLANE_URL`/`CONTROL_PLANE_TOKEN` are set; always writes manifest locally.
- Endpoints: `/health`, `/ready`, `POST /trigger` (optional `WORKER_TOKEN`), `/plan` (GET existing, POST to regenerate).
- Config (env-driven): `QUEUE_BACKEND`, `QUEUE_FILE`, `REDIS_URL`, `REDIS_KEY`, `KAFKA_BROKERS`, `KAFKA_TOPIC`, `OUTPUT_DIR`, `CACHE_DIR`, `PYTHON_VERSION`, `PLATFORM_TAG`, `INDEX_URL`, `EXTRA_INDEX_URL`, `UPGRADE_STRATEGY`, `PLAN_OVERRIDES_JSON`, `MAX_DEPS`, `REQUIREMENTS_PATH`, `CONTAINER_IMAGE`, `CONTAINER_PRESET`, `WORKER_TOKEN`, `CONTROL_PLANE_URL`, `CONTROL_PLANE_TOKEN`, `WORKER_AUTORUN_INTERVAL`, `PODMAN_BIN` (default `podman`), `WORKER_RUN_CMD`, `RUNNER_TIMEOUT_SEC`, `REQUEUE_ON_FAILURE`, `MAX_REQUEUE_ATTEMPTS`, `BATCH_SIZE`, `OBJECT_STORE_*`.
- Metrics: defer Prometheus; keep health/ready.

## MVP flow
1) Pop batch from queue (file/redis/kafka).
2) Load plan: prefer `/output/plan.json`; if absent, plan via the control-plane pending input API (object store + metadata).
3) Match retry requests to plan, apply recipes/overrides.
4) For each job: run build in podman container with mounts; honor timeouts/backoff; capture structured logs (status, reason, elapsed) and stream chunks live. Default command is the Python `refinery --only` invocation; override with `WORKER_RUN_CMD` for custom build drivers.
5) Write manifest to `/output/manifest.json`; post manifest/logs/plan/events to control-plane if configured.
6) Optionally requeue failed jobs when `REQUEUE_ON_FAILURE=true` (bounded by `MAX_REQUEUE_ATTEMPTS`). Expose `/trigger` to drain once; optional autorun interval.

## Components
- `queue`: reuse control-plane interface/backends.
- `runner`: podman exec wrapper (mounts cache/output; sets env PYTHON/PLATFORM tags plus job metadata; supports preset images or custom image; runs `WORKER_RUN_CMD` or a default echo).
- `service`: HTTP server with trigger/health/ready; background autorun optional.
- `reporter`: posts manifest/logs/plan to control-plane.
- `plan`: loader that uses existing plan.json or regenerates it via the Go resolver (respects index settings and upgrade strategy).

## Caveats / Later
- Docker support later.
- Prometheus later.
- Once Go resolver exists, drop Python CLI bridge for plan generation.
