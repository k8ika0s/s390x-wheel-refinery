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
- **Queue backend:** define an interface; file/JSON default, Redis available now, Kafka stubbed for future. Goal: swap backend via config without API changes.
- **History/hints store:** Postgres as default; docker-compose launches Postgres, but allow pointing to external Postgres via env/config. Schema kept portable.
- **Logs/artifacts:** centralized logging/audit tables (not just path refs). Wheel links can point to output/cache; store log records (or references) in DB; add optional text search.
- **Plan storage:** persist the build plan graph in Postgres (JSONB) so UI can fetch the latest plan quickly; worker will write, API will read.
- **Plan storage:** persist the build plan graph in Postgres (JSONB) so UI can fetch the latest plan quickly; worker will write, API will read.
- **Worker trigger:** support both HTTP webhook (POST, token optional) and local trigger. Keep payload compatible with current Python worker (`{action:"drain"}`); optionally support a smoke/dry-run.
- **Worker trigger:** support both HTTP webhook (POST, token optional) and local trigger. Keep payload compatible with current Python worker (`{action:"drain"}`); optionally support a smoke/dry-run. If `WORKER_TOKEN` is set, require `X-Worker-Token`/`token`; otherwise open.
- **API surface:** summary, recent/history search, queue (list/enqueue/clear/stats), worker trigger, hint CRUD, log fetch/search, metrics/health, config view, manifests/artifact links. Build plan exposed, but no “why” reasoning needed.

### First implementation steps (proposed)
1) Define API spec (OpenAPI/JSON schemas) for: summary, recent/history search (pagination/filters), plan, manifest/artifacts, queue ops (list/enqueue/clear/stats), worker trigger (local/webhook), hint CRUD, log fetch/search, metrics/health, config view.
2) Storage wiring: Postgres schema for history/hints/logs/manifests; queue interface with file backend first, Redis/Kafka adapters sketched; log records stored in DB with optional file refs.
3) Implement read-only endpoints first (summary/recent/history, plan/manifest/artifacts, hints read, logs read, metrics/health, config) to unblock UI wiring.
4) Add queue/enqueue/clear/stats and worker trigger (webhook/local) with token stub; expose queue length and oldest age.
5) Add hint CRUD endpoints; record matched hint ids on events; validation for patterns/recipes.
6) Add log search/tail endpoint (basic text search over stored logs).
7) Observability: health/readiness, structured logging, Prometheus metrics (optional, not a blocker). Metrics endpoint is stubbed until Prometheus is wired—documented explicitly so it is not surprising.

### Endpoint draft (coarse)
- `GET /summary`, `/recent`, `/history` (filters: package/status/date/run_id; pagination with sensible defaults, e.g., limit 50, max 500).
- `GET /plan`, `/manifest`, `/artifacts` (links/paths to wheels).
- `GET /queue`, `POST /queue/enqueue`, `POST /queue/clear`, `GET /queue/stats` (length, oldest age).
- `POST /worker/trigger` (local or webhook based on config), optional `POST /worker/smoke`.
- `GET /hints`, `POST/PUT/DELETE /hints/{id}`.
- `GET /logs/{name}/{version}`, `GET /logs/search`.
- `GET /metrics`, `GET /health`, `GET /config`.
Auth: stubbed (open for now); reserve header/query/cookie token for future writes.

### Notes
- No legacy API coexistence needed; this Go service is the first deploy.
- Defaults (pagination/timeouts) to be proposed with the implementation.
- Compose: `podman-compose.yml` brings up Postgres, Redis, Redpanda (Kafka), the Go control-plane, the Go worker (wired to POST plan/manifest/logs back), and the UI pointed at the Go API. File/Redis/Kafka queue backends selectable via env (`QUEUE_BACKEND`).
- Metrics endpoint is stubbed (501) until Prometheus wiring is added.
- Kafka backend does not support a “clear” operation; use Redis/file if you need queue clearing during development.
- Quick start: `podman compose -f podman-compose.yml up` (API :8080, UI :3000). Env overrides: `QUEUE_BACKEND=file|redis|kafka` (default redis), `POSTGRES_DSN`, `REDIS_URL`, `KAFKA_BROKERS`, `WORKER_TOKEN`, `WORKER_WEBHOOK_URL` if you run a remote worker.
