# Project Status — s390x Wheel Refinery

## Overall
- Core pipeline solid: scan/plan/build with adaptive variants, backoff, timeouts, hint-based retries, manifest, history, cache reuse, and container presets.
- Resilience: hint catalog with auto-apply, bounded dependency expansion, retry queue, worker CLI, webhook/local worker modes, token-guarded endpoints, queue CLI, metrics endpoint.
- Observability: SQLite history, logs + SSE streaming, web UI (recent events, failures, variants, hints, queue depth, worker mode), manifest metadata.
- CI/tooling: ruff + pytest + smoke flows; branch merged to main; 19 tests passing locally.

## Completed
- Retry queue + worker (local + webhook) and dedicated worker service (`worker_service.py`).
- Web UI hooks: queue depth, worker trigger; auth guard via header/query/cookie; token cookie helper.
- Docs updated (README/user guide) with compose example, token helper, queue/worker usage; CLI additions (`worker`, `queue`).
- Container presets (Rocky/Fedora/Ubuntu), resource flags, history/serve commands, hint-based resilience.

## Partial / In Progress
- Token distribution depends on operators (reverse-proxy/cookie); no built-in login.
- Metrics endpoint is auth-gated and not Prometheus-formatted; no dashboards wired yet.
- UI is functional but utilitarian; minimal auth beyond token guard.
- Worker autorun only for local mode; webhook mode needs an external scheduler if desired.

## To Do / Near-Term
- Auth: integrate reverse-proxy auth or session-backed login for UI and worker endpoints.
- Packaging: publish/build web and worker images automatically (plus compose/k8s manifests).
- Metrics: expose Prometheus-friendly metrics (queue depth, worker mode, status counts) and provide Grafana examples.
- UX polish: richer package detail (attempt history, hints applied), actionable alerts, improved styling.
- Config UX: sample overrides/hints templates and validation.
- CI: add static type checks (mypy/pyright) and optional integration smoke for worker webhook path.
- Operations: cron/scheduler container to trigger `/trigger` when queue > 0 in webhook mode.

## Wishlist / Future
- Per-package system-recipe library with distro-specific steps and retry heuristics; auto-attach to queue entries.
- Distributed builders: enqueue jobs to separate builder containers with per-build sandboxing and shared cache.
- Smarter resolver: eager/latest-compatible constraint solving and automatic “fallback latest” heuristics.
- ML-focused presets: images with BLAS/Arrow/CUDA stubs for s390x; pre-flight checks for missing libs.
- Manifest/DB exports: Prometheus metrics, CSV/Parquet exports, APIs for “available versions by py tag/platform.”
- UI enhancements: live build console, attempt comparisons by variant, “suggest fix” buttons that write overrides/hints.
- Pluggable storage: S3/MinIO for wheels/logs; migrate SQLite history to Postgres when scaling.
