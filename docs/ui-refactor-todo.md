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

## Next wave (Tailwind/UI polish)
- Adopt Tailwind-based styling with a glassy dark theme, iconography, and consistent spacing. **(in progress)**
- Add sidebar/top nav with environment badge and API base indicator. **(initial header added)**
- Add toasts/snackbars for actions (enqueue, trigger worker, token save). **(done)**
- Add skeleton loaders and better loading states for cards/tables/logs. **(done)**
- Implement pagination/sorting on tables and sticky headers. **(events paginated + sortable + sticky header)**
- Add tabs in package detail (overview, attempts/variants, failures, logs, hints). **(overview/events/hints tabs added; variants/failures still in overview)**
- Add log viewer panel with stream toggle, auto-scroll, and download link. **(initial pass done)**
- Add queue list with items and basic controls; show worker health/status. **(table added)**

## Backend/API support
- API already exposes: summary, recent, top failures/slowest, queue, metrics, logs, retry enqueue, worker trigger.
- No changes needed for API; ensure CORS remains open for the SPA.

## New UX backlog (to implement)
- Build plan visibility: show dependency graph/build plan, pinning vs upgrade strategy, per-node status/reason.
- Artifact access: manifest viewer/download links and wheel download links for built artifacts.
- History browsing: paginate/search history beyond “recent”, filter by package/status/date/run id.
- Cache insight: show cache hits/misses, reused vs rebuilt; add “purge cache for package” (if API support added).
- Hint management: read/write UI for hint catalog (add/edit/delete recipes and notes).
- Worker state: display worker/webhook mode, last run time/result, simple activity log; “run smoke test” button.
- Log ergonomics: search/regex within logs, tail toggle, copy log URL.
- Queue UX: bulk select by status, sort by age, show age of oldest item, quick refresh.
- Config/index visibility: display current indexes/upgrade strategy in effect (read-only).
- Metrics over time: sparklines for success/fail rate and queue depth with trends.
