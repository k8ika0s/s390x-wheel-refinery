# Project Status — s390x Wheel Refinery

## Overall
- Pipeline is now Go-first with planner/control-plane + worker, content-addressed artifacts (Zot), optional wheel mirroring (MinIO), and a dedicated builder image with recipes and auditwheel repair.
- Requirements.txt flow is supported end-to-end: planner emits DAG with pack/runtime edges, worker obeys ordering, mounts packs/runtimes, builds wheels, and publishes wheels/repairs with provenance.
- Observability: control-plane stores manifests/events, UI surfaces queue/artifacts/metrics/logs, Prometheus endpoint is available, and metrics panels are in the UI.
- Resilience: retry queue, token-guarded endpoints, hint catalog, bounded dep expansion, backoff/timeouts, and manifest/history capture.

## Completed
- Go planner emits pack/runtime dependency edges; worker topo-sorts and mounts packs/runtimes before wheel builds.
- CAS/object-store integration (Zot + MinIO) with digest-checked fetch/push, pack/runtime extraction, and repair uploads.
- Builder image (`containers/refinery-builder/Containerfile`) with toolchains, auditwheel/patchelf, and recipes at `/app/recipes`; default worker image/commands point here.
- Recipe book added (packs + CPython runtimes) with pinned SHA256s; default auditwheel-based repair (`recipes/repair.sh`).
- UI/control-plane updates: artifact digests/URLs, metrics panels, queue controls, token cookie helper.
- Docs refreshed: README rewrite, architecture overview, updated diagrams.

## Partial / In Progress
- Pack dependency metadata is hardcoded in planner; needs promotion into a catalog.
- Builder image must be published to registries used by workers (currently local build expected).
- Pack/runtime recipes are available; broader production hardening and SBOM/signing still open.
- Auth remains token-based; no integrated login/revocation story yet.
- Metrics endpoint exists; full Prometheus/Grafana dashboards not shipped.

## To Do / Near-Term
- Publish builder image and wire deployments to consume it.
- Move pack dependency data into a maintained catalog; expand recipes as needed.
- Finalize Prometheus dashboards and alerts (queue depth, worker status, artifact pushes/failures).
- Tighten repair/policy metadata and SBOM/signing options.
- Improve UI polish (more artifact/provenance detail, nicer styling) and auth integration.

## Wishlist / Future
- Scheduler/cron helper for webhook mode to trigger worker when queue > 0.
- Per-package/system pack catalog with distro-specific steps and auto-attach to queue entries.
- Smarter resolver heuristics and richer artifact metadata exports.
- Distributed builders with shared cache and stronger sandboxing.
