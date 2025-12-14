# Project Status — s390x Wheel Refinery

## Overall
- Core pipeline solid: scan/plan/build with adaptive variants, backoff, timeouts, hint-based retries, manifest, history, cache reuse, and container presets.
- Resilience: hint catalog with auto-apply, bounded dependency expansion, retry queue, Go worker (local/webhook), token-guarded endpoints, queue CLI, metrics endpoint (stubbed).
- Observability: Postgres-backed control-plane, logs + SSE streaming, web UI (recent events, failures, variants, hints, queue depth, worker mode), manifest metadata.
- CI/tooling: Go tests (control-plane/worker), ruff + pytest + smoke flows; branch merged to main.

## Completed
- Retry queue + Go worker (local + webhook) and control-plane endpoints for queue/worker.
- Web UI hooks: queue depth, worker trigger; auth guard via header/query/cookie; token cookie helper.
- Docs updated (README/user guide) with compose example, token helper, queue/worker usage; CLI additions (`worker`, `queue`).
- Container presets (Rocky/Fedora/Ubuntu), resource flags, hint-based resilience.

## Partial / In Progress
- Token distribution depends on operators (reverse-proxy/cookie); no built-in login.
- Metrics endpoint is auth-gated and not Prometheus-formatted; no dashboards wired yet.
- UI is functional but utilitarian; minimal auth beyond token guard.
- Worker autorun only for local mode; webhook mode needs an external scheduler if desired.
- New CAS/OCI-based artifact plan captured in `docs/artifact-cas-plan.md` (runtimes/packs/wheels as content-addressed artifacts, Zot + MinIO); implementation not started.

## To Do / Near-Term
- Auth: integrate reverse-proxy auth or session-backed login for UI and worker endpoints.
- Packaging: publish/build web and worker images automatically (plus compose/k8s manifests); builder container refresh.
- Metrics: expose Prometheus-friendly metrics (queue depth, worker mode, status counts) and provide Grafana examples; enrich metadata.
- UX polish: richer package detail (attempt history, hints applied), actionable alerts, improved styling.
- Config UX: sample overrides/hints templates and validation.
- CI: add static type checks (mypy/pyright) and optional integration smoke for worker webhook path.
- Operations: cron/scheduler container to trigger `/trigger` when queue > 0 in webhook mode.
- CAS plan: define artifact keys, DAG schema, pack catalog, runtime/pack build/publish to Zot; worker fetch/mount + wrapper for interpreter selection; control-plane metadata enrichment.

## Wishlist / Future
- Per-package system-recipe library with distro-specific steps and retry heuristics; auto-attach to queue entries.
- Distributed builders: enqueue jobs to separate builder containers with per-build sandboxing and shared cache.
- Smarter resolver: eager/latest-compatible constraint solving and automatic “fallback latest” heuristics; DAG with runtimes/packs/repairs.
- ML-focused presets: images with BLAS/Arrow/CUDA stubs for s390x; pre-flight checks for missing libs; pack catalog expansion.
- Manifest/DB exports: Prometheus metrics, CSV/Parquet exports, APIs for “available versions by py tag/platform.”
- UI enhancements: live build console, attempt comparisons by variant, “suggest fix” buttons that write overrides/hints.
- Pluggable storage: S3/MinIO and OCI CAS for wheels/logs/artifacts; richer provenance and SBOM/signing.
