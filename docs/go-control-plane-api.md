## Go Control Plane API (Draft)

This draft captures the intended endpoints for the Go control plane, matching the agreed design: Postgres for history/hints/logs, pluggable queue backend (file/Redis/Kafka), centralized logging/audit, optional Prometheus, no legacy API overlap, auth stubbed (open for now).

### Conventions
- Base path: `/api` (no legacy v1 coexistence).
- Pagination: `limit` (default 50, max 500), `offset` (default 0).
- Auth: currently open; reserve `X-Worker-Token` header/query/cookie for future write protection.
- Content: JSON responses; errors use `{ "error": "...", "detail": "..." }`.

### Endpoints

**Health/Config/Metrics**
- `GET /health` → `{status:"ok"}`
- `GET /config` → current strategy, target python/platform, index settings, queue backend, db info (sanitized).
- `GET /metrics` → Prometheus if enabled; otherwise 501 (explicitly stubbed until metrics wiring is added).

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

**Plan/Manifest/Artifacts**
- `GET /plan` → current build plan/graph (no “why” reasons).
- `POST /plan` → save plan snapshot (worker writes run_id + plan array to Postgres).
- `GET /manifest?limit=` → manifest JSON for last run (default 200, max 1000).
- `POST /manifest` → save manifest entries (worker writes after build); artifacts are derived from manifest paths/urls.
- `GET /artifacts?limit=` → list of built wheel paths/URLs (default 200, max 1000).

**Config/Backends**
- Queue backend selectable via config (`QUEUE_BACKEND=file|redis|kafka`); file/Redis supported, Kafka implemented (no queue clear); file is default.
- Plan stored in Postgres (JSONB) for quick UI fetch; manifests/logs/history also in Postgres.
- Session helper: `POST /session/token?token=` sets `worker_token` cookie (browser convenience for protected worker/queue actions).

**Queue**
- `GET /queue` → items (package, version, tags, recipes, enqueued_at).
- `GET /queue/stats` → length, oldest age.
- `POST /queue/enqueue` body `{package, version, python_tag, platform_tag, recipes}`.
- `POST /queue/clear` → clear queue (not supported for Kafka backend).

**Worker Trigger**
- `POST /worker/trigger` → drain queue via local or webhook, returns detail + queue length. Honors `X-Worker-Token`/`token` when `WORKER_TOKEN` is set; open otherwise.
- `POST /worker/smoke` (optional) → validate mounts/config without draining. Same token behavior.

**Hints**
- `GET /hints` → list hints.
- `POST /hints` body `{pattern, recipes, note}` → create.
- `PUT /hints/{id}` → update.
- `DELETE /hints/{id}` → delete.

**Logs**
- `GET /logs/{name}/{version}` → log content/metadata.
- `GET /logs/search?q=&limit=` → simple text search over logs.
- `POST /logs` → ingest/store a log entry (name/version/content/timestamp auto-set if omitted).
- `GET /logs/stream/{name}/{version}` → SSE-style single-event stream of the latest log entry.

### Data shapes (coarse)
- Event: `{run_id,name,version,python_tag,platform_tag,status,detail,metadata,timestamp,matched_hint_ids?}`
- Hint: `{id,pattern,recipes:{dnf:[],apt:[]},note}`
- Queue item: `{package,version,python_tag,platform_tag,recipes,enqueued_at}`
- Plan node: `{name,version,python_tag,platform_tag,action:"build"|"reuse"|"skip"}`
- Manifest entry: `{name,version,wheel,python_tag,platform_tag,status}`

### Backends (implementation notes)
- Queue: interface with file backend first; adapters for Redis and Kafka planned; selectable via config.
- DB: Postgres schema for events, hints, logs, manifests; allow external Postgres via env/DSN; compose includes local Postgres.
- Logs: stored in DB (content or compressed text); optional file refs; search is basic text match.
- Worker trigger: webhook payload `{action:"drain"}` with optional token; local trigger runs worker hook if configured.
