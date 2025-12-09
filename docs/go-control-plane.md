## Go Control Plane: Intent and Next Steps (Layperson Overview)

We want a fast, scalable API service in Go that acts as the “traffic controller” for the wheel refinery. In simple terms, this service will: show status to the dashboard, keep track of what’s been built and what’s pending, let us request new builds or retries, and trigger the worker that actually builds wheels. It will talk to storage (for history, hints, queue) and expose clean endpoints the UI and automation can call.

### What it must do (high level)
- Drive the UI: serve JSON for dashboards, history, per-package views, hints, logs, metrics, build plans, manifests, and artifact links.
- Orchestrate work: enqueue retries/builds, expose queue state (length, items, oldest age), trigger worker (local/webhook), and publish the build plan/graph.
- Talk to storage: read/write history/events, manifests, hint catalog (CRUD), queue state; expose cache stats (hit/miss) and log references.
- AuthN/Z: token-based protection for write actions; leave read-only endpoints open or optionally protected.
- Observability: health/readiness, metrics (queue depth, success/fail, cache hit/miss), structured logs, clear error messages.
- Config surfacing: show current strategy (pinned/upgrade), target Python/platform, index settings, environment info.

### Backends and contracts
- **Queue backend:** start with file/JSON for parity, but design to swap to Redis/SQS later (interface-based).
- **History/hints store:** start with SQLite/Postgres-ready schema for events, variants, hints, logs refs, manifests.
- **Logs/artifacts:** return references/URLs; actual blobs can stay on disk/object storage for now.
- **Worker trigger:** HTTP webhook payload compatible with existing Python worker; include token.
- **API schemas:** lock contracts for summary/recent/history search, plan/manifest, queue (list/enqueue/clear), worker trigger, hint CRUD, log fetch/search, config view, metrics.

### First implementation steps (proposed)
1) Define API spec (OpenAPI/JSON schemas) for core endpoints: summary, recent/history search, plan, manifest, queue ops, worker trigger, hint CRUD, log fetch, metrics, config view.
2) Choose initial storage: SQLite for history/hints/manifest; file-backed queue (with interface to swap backends); logs referenced by path/URL.
3) Implement read-only endpoints first (summary/recent/history search/plan/manifest/hints/log refs/metrics/config) to unblock UI wiring.
4) Add queue/enqueue/clear and worker trigger (webhook/local) with token auth on writes; expose queue stats (length, oldest age).
5) Add hint CRUD endpoints; record which hint matched which event in history.
6) Add log search/tail endpoint (basic text search over stored logs).
7) Observability and ops: health/readiness, structured logging, Prometheus metrics.

### Open decisions to align on
- Queue backend: stick to file for now or introduce Redis? (recommend file first, interface ready for Redis).
- DB choice: SQLite vs Postgres for history/hints; start with SQLite, keep schema portable.
- Log storage: keep as file path refs under cache/output, or push to object storage later.
- Auth: continue simple token for write actions; optional role split (read vs write) later.
- API versioning: prefix routes (e.g., /api/v1) to ease rollout alongside the existing Python API.
