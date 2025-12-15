# s390x Wheel Refinery — Friendly Guide

This guide is for anyone (non-technical to semi-technical) who wants to turn any pile of Python wheels into s390x wheels for mainframe use.

## What it does (in plain words)
- Reads the wheels you drop into an input folder.
- Reuses wheels that already work on s390x and rebuilds the rest.
- Learns from failures: suggests missing system packages, tries again with those, and remembers what worked.
- Runs safely in a container so your machine stays clean.
- Shows a web dashboard with statuses, hints, and live logs.

## Quickest paths
1) Make three folders: `/input`, `/output`, `/cache`.
2) Choose your input style:
   - **Wheels path**: drop wheels into `/input`.
   - **requirements.txt**: place a `requirements.txt` in `/input` (unpinned specs will be resolved against the configured index).
3) Run:
```
refinery \
  --input /input \
  --output /output \
  --cache /cache \
  --python 3.11 \
  --container-preset rocky \
  --jobs 1
```
4) If it fails, check the dashboard to see hints and logs.

## Dashboard (React SPA)
Run the Go control-plane and UI:
```
# Compose stack (Postgres/Redis/Redpanda + Go control-plane + UI + Go worker)
podman compose -f docker-compose.control-plane.yml up
# or docker compose -f docker-compose.control-plane.yml up
```
API will be on :8080, UI on :3000. For local UI dev from `ui/`, run `npm install && VITE_API_BASE=http://localhost:8080 npm run dev`. The SPA shows recent events, failures/slow packages, hint catalog, queue depth with items, worker trigger, variant history, per-package detail pages, and log viewing (via `/api/logs/...`). It includes a sticky header with environment/API and token badges, toasts, skeleton states, paginated/sortable event tables with sticky headers, a richer queue table (select, bulk retry, clear), tabs on package detail (overview/events/hints), paginated variants/failures, and log viewers with autoscroll/download controls.

## Key options (plain-English)
- `--python 3.11`: Target Python version for the rebuilt wheels.
- `--container-preset rocky` (or `fedora`/`ubuntu`): Run in a container so the host isn’t changed. Add `--container-cpu`/`--container-memory` to limit resources.
- `--jobs 2`: Build multiple packages at once (after a first warm-up run with `--jobs 1`).
- `--auto-apply-suggestions`: When the tool spots missing libs/headers, it suggests system packages and can auto-try again with them.
- `--fallback-latest`: If the exact version isn’t available for s390x, try the latest compatible one.
- `--skip-known-failures`: Don’t retry packages that already failed.
- `--attempt-timeout`, `--max-attempts`, `--attempt-backoff-base/max`: Control how long/ how many times to try and how long to wait between tries.
- `--schedule shortest-first`: Build shorter packages first using past timing data.
- **requirements.txt support**: Put `requirements.txt` in `/input` (or set `REQUIREMENTS_PATH`) to seed the plan. Optional `CONSTRAINTS_PATH` to force pins. Unpinned specs (`>=`, `~=`) are resolved via your index (`INDEX_URL`/`EXTRA_INDEX_URL`, with `INDEX_USERNAME/PASSWORD` for private repos). Limit dependency expansion with `MAX_DEPS` (default 1000). Per-package overrides via `PLAN_OVERRIDES_JSON`.

## Where things go
- New s390x wheels: `/output`
- Logs: `/cache/logs/...`
- Manifest: `/output/manifest.json` (what happened to each package)
- History database: `/cache/history.db` (used by the dashboard)

## Handling failures
1) Open the dashboard, find the failing package.
2) Read the hint (e.g., “Suggested packages: dnf: openblas-devel | apt: libopenblas-dev”).
3) Re-run with `--auto-apply-suggestions` (or add those packages to your build image).
4) If it keeps failing, use `--skip-known-failures` to move on, then address it later.

## Retry later with the queue
- On a package page in the dashboard, click **Retry with recipe** (or call `POST /api/queue/enqueue`), which drops a request into the queue backend (file/Redis/Kafka, defaults to file/Redis via compose).
- When you’re ready, trigger the Go worker (compose includes one) to consume the queue; the worker applies queued recipes as overrides and rebuilds matching packages. The queue is emptied as it runs.
- The dashboard shows queue length and a **Run worker now** button. It works when the control-plane can reach the worker (local or webhook) and both see `/input`, `/output`, and `/cache`.
- Optional auth: set `WORKER_TOKEN` to require `X-Worker-Token` (or `?token=`) for queue/worker actions.
- CLI queue check: use the Go API `/api/queue`/`/api/queue/stats` or the UI.
- Cookie tip: if you don’t want the token in URLs, set a cookie named `worker_token` in the browser (or call `POST /api/session/token?token=<your_token>`); the UI will send it automatically when triggering the worker.

## Tips for AI/ML stacks
- Use `rocky` preset or a custom image with OpenBLAS/LAPACK and build tools installed.
- Keep `/cache` between runs so successes/hints/logs are remembered.
- Start with `--jobs 1` to warm up, then increase `--jobs` for speed.

## If you want more control
- Inspect `manifest.json` for per-package details (attempt, log path, hints).
- Query history from CLI: `refinery history --db /cache/history.db --recent 20 --top-failures 10 --json`
- Limit container resources: `--container-cpu 2 --container-memory 4g`
- Run tests (for maintainers): `pip install -e .[dev] && pytest -q`
- CI (for maintainers): GitHub Actions runs lint, pytest, container/web smokes, and a dummy-wheel orchestration smoke on PRs.
- Dashboard shows recent failures, hints, and recipes (when available) for troubleshooting.
- Package pages show recent failures and variant history so you can see what worked and what didn’t.

Run the command, watch the dashboard, and collect your s390x wheels. The tool learns from each run to make the next one smoother.
