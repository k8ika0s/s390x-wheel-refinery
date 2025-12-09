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
- `GET /metrics` → Prometheus if enabled; otherwise 404/501.

**Summary/History**
- `GET /summary` → status counts (recent window), recent failures list.
- `GET /recent?package=&status=&limit=&offset=` → latest events.
- `GET /history?package=&status=&run_id=&from=&to=&limit=&offset=` → paginated history.
- `GET /package/{name}` → package summary (counts + latest).
- `GET /event/{name}/{version}` → last event for that version.
- `GET /failures?name=&limit=` → failures over time for a package.
- `GET /variants/{name}?limit=` → variant history for a package.
- `GET /top-failures?limit=` / `GET /top-slowest?limit=` → stats.

**Plan/Manifest/Artifacts**
- `GET /plan` → current build plan/graph (no “why” reasons).
- `GET /manifest` → manifest JSON for last run.
- `GET /artifacts` → list of built wheel paths/URLs.

**Queue**
- `GET /queue` → items (package, version, tags, recipes, enqueued_at).
- `GET /queue/stats` → length, oldest age.
- `POST /queue/enqueue` body `{package, version, python_tag, platform_tag, recipes}`.
- `POST /queue/clear` → clear queue.

**Worker Trigger**
- `POST /worker/trigger` → drain queue via local or webhook, returns detail + queue length.
- `POST /worker/smoke` (optional) → validate mounts/config without draining.

**Hints**
- `GET /hints` → list hints.
- `POST /hints` body `{pattern, recipes, note}` → create.
- `PUT /hints/{id}` → update.
- `DELETE /hints/{id}` → delete.

**Logs**
- `GET /logs/{name}/{version}` → log content/metadata.
- `GET /logs/search?q=&limit=` → simple text search over logs.

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
