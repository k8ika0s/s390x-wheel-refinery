## Go Control Plane API (Draft)

This draft captures the intended endpoints for the Go control plane, matching the agreed design: Postgres for history/hints/logs, pluggable queue backend (file/Redis/Kafka), centralized logging/audit, optional Prometheus, no legacy API overlap, auth stubbed (open for now).

### Conventions
- Base path: `/api` (no legacy v1 coexistence).
- Pagination: `limit` (default 50, max 500), `offset` (default 0).
- Auth: `UI_TOKEN` protects user-initiated writes; `WORKER_TOKEN` protects worker posts.
- Content: JSON responses; errors use `{ "error": "...", "detail": "..." }`.

### Endpoints

**Health/Config/Metrics**
- `GET /health` → `{status:"ok"}`
- `GET /ready` → readiness (DB/queue reachable).
- `GET /config` → current strategy, target python/platform, index settings, queue backend, db info (sanitized).
- `GET /metrics` → Prometheus-style counters and gauges for retries, worker readiness, package-unavailable failures, dependency-pack fallback, degraded-build outcomes, and builder-profile usage.

**Summary/History**
- `GET /summary` → status counts (recent window), recent failures list.
- `GET /summary?failure_limit=` → status counts plus latest failures (default 20).
- `GET /recent?package=&status=&limit=&offset=` → latest events.
- `GET /history?package=&status=&run_id=&from=&to=&limit=&offset=` → paginated history.
- `GET /package/{name}` → package summary (counts + latest).
- `GET /event/{name}/{version}` → last event for that version.
- `GET /failures?name=&limit=` → failures over time for a package.
- `GET /variants/{name}?limit=` → variant history for a package.
- `GET /top-failures?limit=` / `GET /top-slowest?limit=` → stats.

**Builds**
- `GET /builds?status=&plan_id=&package=&version=&limit=` → build status rows.
- `GET /builds/attempts?package=&version=&limit=` → per-attempt history for a package/version, including remediation metadata such as builder profile, remediation tier, missing packages, pack requirements, and effective pack mounts.
- `POST /builds/status` → worker status updates (attempts/backoff/failure metadata, optional `worker_id`).
- `POST /build-queue/pop` → lease build items (worker; accepts optional `X-Worker-Id` header).
- `POST /build-queue/requeue-stale` → requeue stale leases/building items.

**Plan/Manifest/Artifacts**
- `GET /plan` → current build plan/graph (no “why” reasons).
- `POST /plan` → save plan snapshot (worker writes run_id + plan array to Postgres).
- `GET /manifest?limit=` → manifest JSON for last run (default 200, max 1000), including manifest_digest and metadata.
- `POST /manifest` → save manifest entries (worker writes after build); artifacts are derived from manifest paths/urls and immutability is enforced by manifest_digest.
- `GET /artifacts?limit=` → list of built wheel paths/URLs (default 200, max 1000).

**Config/Backends**
- Queue backend selectable via config (`QUEUE_BACKEND=file|redis|kafka`); file/Redis supported, Kafka implemented (no queue clear); file is default.
- Plan stored in Postgres (JSONB) for quick UI fetch; manifests/logs/history also in Postgres.
- Session helpers: `POST /session/ui-token?token=` sets `ui_token` cookie; `POST /session/token?token=` sets `worker_token`.

**Python versions/recipes**
- `GET /python-versions` → list managed python versions from the recipes directory.
- `GET /python-versions/{version}` → fetch recipe content/metadata for a version.
- `PUT /python-versions/{version}` → create/update a recipe (JSON: `{ recipe: "..." }`, requires UI token).

**Queue**
- `GET /queue` → items (package, version, tags, recipes, enqueued_at).
- `GET /queue/stats` → length, oldest age.
- `POST /queue/enqueue` body `{package, version, python_tag, platform_tag, recipes}` (requires UI token).
- `POST /queue/clear` → clear queue (not supported for Kafka backend, requires UI token).

**Worker Trigger**
- `POST /worker/trigger` → drain queue via local or webhook, returns detail + queue length. Honors `X-UI-Token` when `UI_TOKEN` is set; open otherwise.
- `POST /worker/heartbeat` → upsert worker status (worker_id, pools, active builds). Requires `X-Worker-Token` when configured.
- `GET /workers` → list worker heartbeat statuses and last-seen timestamps.
- `POST /worker/smoke` (optional) → validate mounts/config without draining. Same token behavior.
- Worker heartbeat metadata includes config readiness, inference/runtime provenance, and configured builder profiles/images.

**Hints**
- `GET /hints` → list hints.
- `POST /hints` body `{pattern, recipes, note}` → create (requires UI token).
- `PUT /hints/{id}` → update (requires UI token).
- `DELETE /hints/{id}` → delete (requires UI token).

**Logs**
- `GET /logs/{name}/{version}` → log content/metadata (latest stored entry); `?raw=1` returns plain text.
- `GET /logs/search?q=&limit=` → simple text search over logs.
- `POST /logs` → ingest/store a full log entry (name/version/content/timestamp auto-set if omitted; requires worker token).
- `GET /logs/chunks/{name}/{version}?after=&after_seq=&attempt=&limit=` → list stored log chunks (for replay); `tail=1` returns the newest chunks; `attempt` scopes a single build attempt.
- `POST /logs/stream/{name}/{version}?attempt=&run_id=` → worker streaming ingest (NDJSON chunks).
- `GET /logs/stream/{name}/{version}?after=&after_seq=&attempt=&limit=` → WebSocket stream of log chunks (live tail, attempt-scoped).
- `GET /simple` → HTML root index of wheelhouse packages (PEP 503).
- `GET /simple/{name}` → HTML package index listing wheel files (PEP 503).

### Data shapes (coarse)
- Event: `{run_id,name,version,python_tag,platform_tag,status,detail,metadata,timestamp,matched_hint_ids?}` where `metadata` may include `builder_profile`, `remediation_tier`, `missing_packages`, `pack_requirements`, `pack_resolution_result`, and `effective_pack_mounts`.
- Hint: `{id,pattern,recipes:{dnf:[],apt:[]},note}`
- Queue item: `{package,version,python_tag,platform_tag,recipes,enqueued_at}`
- Plan node: `{name,version,python_tag,platform_tag,action:"build"|"reuse"|"skip"}`
- Manifest entry: `{name,version,wheel,python_tag,platform_tag,status}`

### Backends (implementation notes)
- Queue: interface with file backend first; adapters for Redis and Kafka planned; selectable via config.
- DB: Postgres schema for events, hints, logs, manifests; allow external Postgres via env/DSN; compose includes local Postgres.
- Logs: stored in DB (content or compressed text); optional file refs; search is basic text match.
- Worker trigger: webhook payload `{action:"drain"}` with optional token; local trigger runs worker hook if configured.
