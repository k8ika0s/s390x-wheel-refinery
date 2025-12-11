# Go Worker Service (Draft)

## Purpose (Layperson)
A fast, container-isolated worker that drains the retry/build queue and rebuilds wheels. It mounts `/input`, `/output`, and `/cache`, runs builds in a Podman container (Docker later), and reports results (plan/manifest/logs/events) to the Go control-plane. This replaces the Python worker path entirely.

## Scope
- Queue backends: file/JSON, Redis, Kafka (same interface as control-plane). No dependency on Python queue.
- Builds: run via Podman with cache/output bind-mounts; presets (rocky/fedora/ubuntu) or custom image. Runner passes job context as env (`JOB_NAME`, `JOB_VERSION`, `PYTHON_TAG`, `PLATFORM_TAG`, optional `RECIPES`) and runs `WORKER_RUN_CMD` if provided; otherwise defaults to `refinery-build` (script in builder images) which invokes `refinery --input /input --output /output --cache /cache --python $PYTHON_TAG --platform-tag $PLATFORM_TAG --only $JOB_NAME==$JOB_VERSION --jobs 1` inside the container. Podman-first; Docker is optional. Stubbed unless `PODMAN_BIN` is set.
- Plan generation: loads `plan.json` from /output or /cache; if missing, uses the Go resolver to scan `/input` wheels and generate `plan.json` (no Python dependency). Resolver respects `INDEX_URL`, `EXTRA_INDEX_URL`, and `UPGRADE_STRATEGY` (`pinned` or `eager`).
- Reporting: posts manifest/logs/plan and events to the Go control-plane when `CONTROL_PLANE_URL`/`CONTROL_PLANE_TOKEN` are set; always writes manifest locally.
- Endpoints: `/health`, `/ready`, `POST /trigger` (optional `WORKER_TOKEN`), `/plan` (GET existing, POST to regenerate).
- Config (env-driven): `QUEUE_BACKEND`, `QUEUE_FILE`, `REDIS_URL`, `REDIS_KEY`, `KAFKA_BROKERS`, `KAFKA_TOPIC`, `INPUT_DIR`, `OUTPUT_DIR`, `CACHE_DIR`, `PYTHON_VERSION`, `PLATFORM_TAG`, `INDEX_URL`, `EXTRA_INDEX_URL`, `UPGRADE_STRATEGY`, `PLAN_OVERRIDES_JSON`, `CONTAINER_IMAGE`, `CONTAINER_PRESET`, `WORKER_TOKEN`, `CONTROL_PLANE_URL`, `CONTROL_PLANE_TOKEN`, `WORKER_AUTORUN_INTERVAL`, `PODMAN_BIN` (default `podman`), `WORKER_RUN_CMD`, `RUNNER_TIMEOUT_SEC`, `REQUEUE_ON_FAILURE`, `MAX_REQUEUE_ATTEMPTS`, `BATCH_SIZE`.
- Metrics: defer Prometheus; keep health/ready.

## MVP flow
1) Pop batch from queue (file/redis/kafka).
2) Load plan: prefer `/output/plan.json`; if absent, rescan and build the plan via the Go resolver (no Python bridge).
3) Match retry requests to plan, apply recipes/overrides.
4) For each job: run build in podman container with mounts; honor timeouts/backoff; capture structured logs (status, reason, elapsed). Default command is the Python `refinery --only` invocation; override with `WORKER_RUN_CMD` for custom build drivers.
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
