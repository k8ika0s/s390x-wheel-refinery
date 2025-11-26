# s390x Wheel Refinery

Refine arbitrary Python wheels into a coherent, reproducible set of s390x wheels. Drop wheels from any platform into `/input`, get rebuilt s390x wheels plus a manifest, live logs, and a history database out of `/output`.

## What it does
- **Scan & plan**: Parse input wheels, reuse pure/compatible ones, and plan rebuilds for the rest (pinned, eager, or fallback-latest).
- **Resilient builds**: Adaptive build variants (ordered by past success), timeouts, exponential backoff, per-attempt logs, and hint-derived recipe retries.
- **Dynamic recovery**: Catalog-driven hints (dnf/apt) for missing libs/headers (AI/ML-friendly), auto-apply suggestions, and bounded dependency expansion to auto-build missing Python deps.
- **Isolation & control**: Run in containers (Rocky/Fedora/Ubuntu presets or custom images) with CPU/mem limits; cache/output bind-mounted to keep the host clean.
- **Concurrency & scheduling**: Parallel builds with warmed cache/venv; shortest-first scheduling using observed durations.
- **Observability**: Manifest with rich metadata, SQLite history of all events, logs on disk, SSE log streaming, and a FastAPI/Jinja control panel (charts, catalog, alerts).

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
- Logging: `--verbose`

## Pipeline
1) **Scan** wheels and classify reuse vs rebuild.
2) **Plan** deps (pinned/eager/fallback-latest), skip known failures, and auto-plan missing Python deps (bounded).
3) **Build** jobs with adaptive variants, timeouts, backoff, logs, hints, and hint-based retry; cache outputs and copy to `/output`.
4) **Emit** manifest (per-entry metadata: variant, attempt, log, duration, hints) and record every event in history.

## Containers
- Builder bases: `containers/rocky/Dockerfile`, `containers/fedora/Dockerfile`, `containers/ubuntu/Dockerfile`
- Web/control-plane: `containers/web/Dockerfile`
- Make targets: `make build-rocky|build-fedora|build-ubuntu|build-web TAG=latest REGISTRY=local`
- Serve UI: `docker run -p 8000:8000 -v /cache:/cache local/refinery-web:latest` (expects `HISTORY_DB=/cache/history.db`).

## Web UI / API
- Start: `refinery serve --db /cache/history.db --host 0.0.0.0 --port 8000`
- Features: recent events, top failures, top slow packages (avg duration + failures), package pages (alerts, avg duration), hint catalog, log links, SSE streaming (`/logs/{name}/{version}/stream`), APIs for stats (`/api/top-failures`, `/api/top-slowest`, `/api/hints`).
- Package pages also show recent failures (with hints/recipes) and variant history; dashboard shows recent failures and status counts.
- Retry queue: POST `/package/{name}/retry` (UI button or API) stores a request in `<cache>/retry_queue.json` with suggested recipes. Run `refinery worker ...` to consume the queue and rebuild those packages.
- Queue visibility & trigger: UI shows queue depth and offers “Run worker now,” which calls `/api/worker/trigger` (enabled when the web container has access to `/input`, `/output`, `/cache` or a worker webhook is configured).
- Auth: optional `WORKER_TOKEN` env protects queue/worker endpoints (supply via `X-Worker-Token` or `?token=` when invoking).
- Queue CLI: `refinery queue --cache /cache` prints queue length; `--queue-path` overrides the default.
- Token cookie helper: `POST /api/session/token?token=<WORKER_TOKEN>` sets a `worker_token` cookie for the browser, avoiding query/header injection in the UI.
- Worker service container (example docker-compose):
  ```yaml
  services:
    refinery-web:
      image: <your-web-image>
      environment:
        HISTORY_DB: /cache/history.db
        WORKER_WEBHOOK_URL: http://worker:9000/trigger
        WORKER_TOKEN: supersecret
      volumes:
        - /input:/input:ro
        - /output:/output
        - /cache:/cache
    worker:
      image: <your-web-image>  # or dedicated worker image
      command: ["uvicorn", "s390x_wheel_refinery.worker_service:app", "--host", "0.0.0.0", "--port", "9000"]
      environment:
        HISTORY_DB: /cache/history.db
        WORKER_TOKEN: supersecret
      volumes:
        - /input:/input:ro
        - /output:/output
        - /cache:/cache
  ```

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
- `refinery worker --input ... --output ... --cache ... --python 3.11` consumes the queue using the same history/cache, applies the queued recipes as overrides, and rebuilds matching jobs.
- Useful for follow-up retries after triage without re-running the full pipeline; the queue is emptied atomically when processed.
- Web-triggered worker: When the web container is given mounts for `/input`, `/output`, and `/cache` (or overrides via `WORKER_*` env vars), the UI “Run worker now” button and `/api/worker/trigger` will run a queue drain in-process. Optional `WORKER_AUTORUN_INTERVAL` (seconds) enables periodic drains.
- Remote worker service: Set `WORKER_WEBHOOK_URL` (and `WORKER_TOKEN`) in the web container to call a remote worker service instead of running locally. Start the worker service with `uvicorn s390x_wheel_refinery.worker_service:app --host 0.0.0.0 --port 9000` in a container that mounts `/input`, `/output`, and `/cache`.

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
