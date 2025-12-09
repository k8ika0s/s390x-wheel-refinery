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
- FastAPI app with CORS enabled, summary/recent/failures/variants/logs/queue/metrics endpoints; worker trigger + session token; queue clear and retry endpoints.
- Auth via worker token header/query/cookie.

**Next steps / gaps**
- Add endpoints for build plan/manifest download and wheel artifacts.
- Add hint catalog CRUD endpoints.
- Add history search/pagination, cache/worker insight endpoints, and log search/tail support.
