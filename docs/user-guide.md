# s390x Wheel Refinery — Friendly Guide

This guide is for anyone (non-technical to semi-technical) who wants to turn any pile of Python wheels into s390x wheels for mainframe use.

## What it does (in plain words)
- Reads the wheels you drop into an input folder.
- Reuses wheels that already work on s390x and rebuilds the rest.
- Learns from failures: suggests missing system packages, tries again with those, and remembers what worked.
- Runs safely in a container so your machine stays clean.
- Shows a web dashboard with statuses, hints, and live logs.

## Quickest path
1) Make three folders: `/input` (your wheels), `/output` (results), `/cache` (for history/logs).
2) Run:
```
refinery \
  --input /input \
  --output /output \
  --cache /cache \
  --python 3.11 \
  --container-preset rocky \
  --jobs 1
```
3) If it fails, check the dashboard (React SPA) to see hints and logs.

## Dashboard (React SPA)
Start the API with:
```
refinery serve --db /cache/history.db --host 0.0.0.0 --port 8000
```
Then run the React dashboard pointing at that API. From `ui/`, run `npm install && VITE_API_BASE=http://localhost:8000 npm run dev` for local work, or build the production UI container in `containers/ui/Dockerfile` (served on port 3000 by default). The SPA shows recent events (reused, built, failed), top failures and slow packages, hint catalog, queue depth with items, worker trigger, variant history, per-package detail pages, and log viewing (via `/logs/...`). The UI now includes a sticky header with environment/API and token badges, toasts for actions, skeleton states while loading, paginated/sortable event tables with sticky headers, a richer queue table (select, bulk retry, clear), tabs on package detail (overview/events/hints), paginated variants/failures, and log viewers with autoscroll/download controls.

## Key options (plain-English)
- `--python 3.11`: Target Python version for the rebuilt wheels.
- `--container-preset rocky` (or `fedora`/`ubuntu`): Run in a container so the host isn’t changed. Add `--container-cpu`/`--container-memory` to limit resources.
- `--jobs 2`: Build multiple packages at once (after a first warm-up run with `--jobs 1`).
- `--auto-apply-suggestions`: When the tool spots missing libs/headers, it suggests system packages and can auto-try again with them.
- `--fallback-latest`: If the exact version isn’t available for s390x, try the latest compatible one.
- `--skip-known-failures`: Don’t retry packages that already failed.
- `--attempt-timeout`, `--max-attempts`, `--attempt-backoff-base/max`: Control how long/ how many times to try and how long to wait between tries.
- `--schedule shortest-first`: Build shorter packages first using past timing data.

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
- On a package page in the dashboard, click **Retry with recipe** (or call `POST /package/{name}/retry`), which drops a request into `/cache/retry_queue.json`.
- When you’re ready, run the worker to consume the queue (uses the same cache/history):
```
refinery worker \
  --input /input \
  --output /output \
  --cache /cache \
  --python 3.11
```
- The worker applies the queued recipe steps as overrides and rebuilds the matching packages. The queue is emptied after each worker run.
- In the dashboard you’ll also see the queue length and a **Run worker now** button. It works when the API container can see `/input`, `/output`, and `/cache` (or when `WORKER_*` env vars point to them). You can also enable a periodic drain with `WORKER_AUTORUN_INTERVAL` (seconds) in local mode.
- If you prefer an external worker: run `uvicorn s390x_wheel_refinery.worker_service:app --host 0.0.0.0 --port 9000` in a container that has the same mounts, then set `WORKER_WEBHOOK_URL=http://worker:9000/trigger` (and optionally `WORKER_TOKEN`) in the API container. The UI “Run worker now” will call the webhook instead of running locally.
- Optional auth: set `WORKER_TOKEN` to require `X-Worker-Token` (or `?token=`) for queue/worker actions.
- CLI queue check: `refinery queue --cache /cache` shows queue length (use `--queue-path` to override).
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

Run the command, watch the dashboard, and collect your s390x wheels. The tool learns from each run to make the next one smoother.***
