# Go Worker Service (Draft)

## Purpose (Layperson)
A fast, container-isolated worker that drains the retry/build queue and rebuilds wheels. It mounts `/input`, `/output`, and `/cache`, runs builds in a Podman container (Docker later), and reports results (plan/manifest/logs/events) to the Go control-plane. This replaces the Python worker path entirely.

## Scope
- Queue backends: file/JSON, Redis, Kafka (same interface as control-plane). No dependency on Python queue.
- Builds: run via Podman with cache/output bind-mounts; presets (rocky/fedora/ubuntu) or custom image. Docker support later. Runner passes job context as env (`JOB_NAME`, `JOB_VERSION`, `PYTHON_TAG`, `PLATFORM_TAG`, optional `RECIPES`) and runs the provided `WORKER_RUN_CMD` (or a simple default echo). Current runner is stubbed unless `PODMAN_BIN` is set.
- Plan fallback: loads `plan.json` from /output or /cache; if missing, shells out to the Python CLI (plan-only) to generate one (temporary bridge until Go resolver exists).
- Reporting: posts manifest/logs/plan and events to the Go control-plane when `CONTROL_PLANE_URL`/`CONTROL_PLANE_TOKEN` are set; always writes manifest locally.
- Endpoints: `/health`, `/ready`, `POST /trigger` (optional `WORKER_TOKEN`).
- Env: `QUEUE_BACKEND`, `QUEUE_FILE`, `REDIS_URL/REDIS_KEY`, `KAFKA_BROKERS/KAFKA_TOPIC`, `INPUT_DIR/OUTPUT_DIR/CACHE_DIR`, `PYTHON_VERSION`, `PLATFORM_TAG`, `CONTAINER_IMAGE/CONTAINER_PRESET`, `WORKER_TOKEN`, `CONTROL_PLANE_URL/TOKEN`, `PODMAN_BIN`, `WORKER_RUN_CMD`, `RUNNER_TIMEOUT_SEC`, `BATCH_SIZE`.
- Reporting: POST manifest/logs/plan/events to the Go control-plane when configured; otherwise write to disk only.
- Endpoints: `/health`, `/ready`, `POST /trigger` (token optional) to kick off a drain. Optional autorun interval flag/env.
- Config (env-driven): `QUEUE_BACKEND`, `QUEUE_FILE`, `REDIS_URL`, `REDIS_KEY`, `KAFKA_BROKERS`, `KAFKA_TOPIC`, `INPUT_DIR`, `OUTPUT_DIR`, `CACHE_DIR`, `PYTHON_VERSION`, `PLATFORM_TAG`, `CONTAINER_IMAGE`, `CONTAINER_PRESET`, `WORKER_TOKEN`, `CONTROL_PLANE_URL`, `CONTROL_PLANE_TOKEN`, `WORKER_AUTORUN_INTERVAL`, `PODMAN_BIN` (default `podman`).
- Metrics: defer Prometheus; keep health/ready.

## MVP flow
1) Pop batch from queue (file/redis/kafka).
2) Load plan: prefer `/output/plan.json`; if absent, rescan+build plan via existing Python CLI as bridge (later reimplement resolver in Go).
3) Match retry requests to plan, apply recipes/overrides.
4) For each job: run build in podman container with mounts; honor timeouts/backoff; capture duration/logs.
5) Write manifest to `/output/manifest.json`; post manifest/logs/plan/events to control-plane if configured.
6) Expose `/trigger` to drain once; optional autorun interval.

## Components
- `queue`: reuse control-plane interface/backends.
- `runner`: podman exec wrapper (mounts cache/output; sets env PYTHON/PLATFORM tags plus job metadata; supports preset images or custom image; runs `WORKER_RUN_CMD` or a default echo).
- `service`: HTTP server with trigger/health/ready; background autorun optional.
- `reporter`: posts manifest/logs/plan to control-plane.
- `plan`: loader that uses existing plan.json or shells out to Python `refinery` CLI to get a plan (temporary bridge).

## Caveats / Later
- Docker support later.
- Prometheus later.
- Once Go resolver exists, drop Python CLI bridge for plan generation.
