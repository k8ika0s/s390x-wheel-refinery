# UI Refactor To-Do (React SPA)

## Goals
- Replace the minimal single-page dashboard with a full control plane UI: package drilldowns, logs, hints, queue visibility, worker status, filters, and real-time updates.

## Must-haves
1) Package detail views: route to `/package/:name` showing history (recent events), variant attempts, failures, and associated hints.
2) Log viewer: stream logs from `/logs/{name}/{version}/stream`, with a simple tail and a download/open link.
3) Hint visibility: list hint catalog and surface hints attached to failures in tables/detail panels.
4) Filters/search/pagination: filter recent events by package/status, paginate or lazy-load event lists.
5) Real-time updates: lightweight polling or SSE for queue length and recent events.
6) Queue management: show queued items (length and basic metadata), allow “run worker now,” and display worker mode/status (local/webhook).
7) Auth UX: token entry/storage with feedback; handle 403s gracefully; optional login page stub if using reverse-proxy auth.
8) Routing/layout: SPA routes for dashboard and package detail; shared header/nav and consistent cards/tables.
9) Metrics/observability: render `/api/metrics` (status counts, failures, queue length, worker mode) with small charts or stats.

## Nice-to-haves
- Dark/light theme toggle; improved styling and spacing.
- Bulk actions: enqueue multiple retries; clear queue.
- Export links: manifest/history CSV/JSON links; log download button.
- Admin banner for webhook/local mode and token state.

## Backend/API support
- API already exposes: summary, recent, top failures/slowest, queue, metrics, logs, retry enqueue, worker trigger.
- No changes needed for API; ensure CORS remains open for the SPA.
