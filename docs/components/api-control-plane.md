The API/control plane is the refinery’s “traffic controller.” It exposes HTTP endpoints that let the UI (and other tools) see what’s happening, trigger work, and fetch logs/metrics. To a layperson: it’s the single front door that shows status, accepts retry requests, and can start the worker.

**Purpose / Responsibilities**
- Serve status (summary, recent events, failures/slowest, metrics).
- Serve history slices (variants, failures, events) and logs.
- Manage queue (list, clear, enqueue retry) and trigger the worker (local or webhook).
- Handle optional token auth for protected actions.

**Why it matters**
- Central, scriptable interface for observability and control.
- Powers the web UI and any external automation/monitoring.

**Fit in the system**
- Sits between storage (history/queue) and clients (UI/CLI).
- Orchestrates worker triggers and exposes CORS-enabled endpoints for the SPA.

**Current status**
- Go HTTP service with CORS enabled; Postgres-backed history/logs/manifest/plan; queue backends (file/Redis/Kafka) selected via config.
- Endpoints: summary/recent/history slices (failures/variants/package), logs (get/search/stream), plan (get/save/compute via worker), manifest/artifacts, queue (list/stats/enqueue/clear), worker trigger/smoke, metrics stub, session token helper.
- Auth via worker token header/query/cookie for write/queue actions.

**Next steps / gaps**
- Improve metrics (Prometheus), pagination/search UX, and cache/worker insight endpoints.
