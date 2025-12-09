# s390x Wheel Refinery — User Guide

This guide explains, in plain language, how to turn a pile of Python wheels into s390x wheels, how the system is put together, and how to run it end-to-end. You do not need to be deeply technical; most steps are copy/paste commands with brief explanations.

## What the refinery does

Think of the refinery as a careful sorter and rebuilder. You drop wheels into an input directory. It keeps any wheels that are already s390x-compatible and rebuilds the rest for the target Python version and platform. When a build fails because a system library is missing, it reads the error, suggests (and can auto-apply) the right packages to install, and tries again. Every build and failure is recorded in a SQLite history database. Logs are saved to disk, and a web dashboard shows what’s happening in near real time.

## Core components and how they work together

- **CLI runner (`refinery`)** scans the input directory, plans what to rebuild, and executes builds in containers. It writes new wheels to the output directory and saves metadata in a manifest and in the history database.
- **Web UI** is a FastAPI/Jinja app that reads the history database, shows recent events, failures, variant history, hints, and lets you enqueue retries with one click. It streams logs when available.
- **Retry queue + worker** let you defer retries. The UI or API enqueues retry requests. A worker (local or remote via webhook) consumes the queue and rebuilds those packages using shared cache/history.
- **Cache/output/input dirs** are shared volumes. Input is read-only wheels; output stores newly built wheels; cache stores sdists, virtualenv, logs, and the history database.
- **Container presets** (Rocky/Fedora/Ubuntu) give you a clean build environment so the host stays untouched; you can also point to a custom image.

## Running the CLI directly

Prepare three directories: `/input` (your wheels), `/output` (results), `/cache` (persistent cache + history). Then run:

```bash
refinery \
  --input /input \
  --output /output \
  --cache /cache \
  --python 3.11 \
  --container-preset rocky \
  --jobs 1
```

If something fails, the exit code will be non-zero. Check `/cache/logs` for logs and the dashboard (see below) for hints.

Key options you might tweak: `--jobs` for parallelism (start with 1, then increase), `--auto-apply-suggestions` to let hints add system packages/recipes automatically, `--fallback-latest` to try the latest compatible version when pinned fails, and `--skip-known-failures` to avoid repeating known failures. Timeouts/backoff/attempts are configurable (`--attempt-timeout`, `--max-attempts`, `--attempt-backoff-base/max`). Resource limits for containers use `--container-cpu` and `--container-memory`.

## Web dashboard

Start the dashboard with:

```bash
refinery serve --db /cache/history.db --host 0.0.0.0 --port 8000
```

Open `http://localhost:8000`. You’ll see recent events (reused, built, failed), top failures and slow packages, package pages with average durations, alerts, and log links. The hint catalog is visible for common missing libraries. There is live log streaming per package/version when logs are available. Package pages also show variant history and recent failures. A queue widget shows how many retry requests are waiting and lets you trigger the worker.

## Retry queue and worker

From the UI, use “Retry with recipe” on a package page to enqueue a retry. The request is stored in `/cache/retry_queue.json` along with any hint-derived recipe steps. To process the queue locally, run:

```bash
refinery worker \
  --input /input \
  --output /output \
  --cache /cache \
  --python 3.11
```

The worker applies queued recipes as overrides and rebuilds matching packages, then clears the queue. The UI “Run worker now” button calls `/api/worker/trigger`; this works when the web container can see `/input`, `/output`, and `/cache`, or when `WORKER_*` env vars point to those paths. You can enable periodic drains in local mode with `WORKER_AUTORUN_INTERVAL` (seconds).

For remote workers, run:

```bash
uvicorn s390x_wheel_refinery.worker_service:app --host 0.0.0.0 --port 9000
```

in a container that mounts `/input`, `/output`, and `/cache`, then set `WORKER_WEBHOOK_URL=http://worker:9000/trigger` (and `WORKER_TOKEN`) in the web container. The UI and `/api/worker/trigger` will call the webhook instead of running locally.

Auth is a simple token guard: set `WORKER_TOKEN` to require `X-Worker-Token`, `?token=`, or a `worker_token` cookie. You can set the cookie via `POST /api/session/token?token=<your_token>`. The queue length is visible via `refinery queue --cache /cache` or `/api/metrics`.

## Deployment with Docker Compose

The repository includes `docker-compose.yml` that starts both web and worker services. Create `./input`, `./output`, and `./cache` on your host. Then run:

```bash
WORKER_TOKEN=changeme docker compose up --build
```

The web UI will be at `http://localhost:8000`; the worker webhook at `http://localhost:9000` (token-protected). Both containers share the same volumes; history lives at `./cache/history.db`. Adjust `WORKER_TOKEN` and port bindings to fit your environment. For production, place a reverse proxy in front of the web/worker services to handle TLS and authentication.

## Where your data lives

New s390x wheels are written to `/output`. Logs are under `/cache/logs/...`. The manifest of what happened in the last run is `/output/manifest.json`. The history database that powers the dashboard lives at `/cache/history.db`. The retry queue is `/cache/retry_queue.json`. Keeping `/cache` between runs allows the system to remember past successes, hints, and timings.

## Handling failures

When a build fails, open the dashboard, find the package, and read any hint. For example, “dnf: openblas-devel | apt: libopenblas-dev” means you can rerun with `--auto-apply-suggestions` or bake those packages into your build image. If a package keeps failing, use `--skip-known-failures` to continue the rest, then revisit with the retry queue once you’ve adjusted recipes or overrides.

## Tips for AI/ML stacks

AI/ML builds often need BLAS/Arrow/compression libs. Start with the `rocky` preset or a custom image that includes OpenBLAS/LAPACK and standard build tools. Run once with `--jobs 1` to warm up the cache, then increase `--jobs` for speed. Keep `/cache` persisted so learned hints and timings are reused.

## Advanced control

Inspect `manifest.json` for per-package details such as attempt number, log path, and applied hints. Query history from the CLI with `refinery history --db /cache/history.db --recent 20 --top-failures 10 --json`. Adjust container resources with `--container-cpu` and `--container-memory`. Maintainers can run tests via `pip install -e .[dev] && pytest -q`; the CI pipeline runs lint, pytest, container/web smokes, and a dummy-wheel orchestration smoke on PRs.

## Mental model to keep in mind

The refinery is deliberately conservative and repeatable. It plans builds based on the wheels you give it, retries with bounded attempts and backoff, and learns from hints without wandering endlessly. The queue lets you triage and try again later. The dashboard gives you visibility into what worked and what needs attention. If you keep the cache, each run becomes faster and more informed. When in doubt, read the history, check the hints, and retry with a small tweak. You’ll build up a reliable library of s390x wheels for your Python workloads. 
