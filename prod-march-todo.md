# prod-march-todo.md

This is the production march checklist. Items are grouped and ordered so that
finishing the list yields end-to-end readiness, then full production readiness.

Legend:
- P0 = required for reliable end-to-end testing
- P1 = required for production readiness
- P2 = polish / scale-up / above-and-beyond

## P0: End-to-end correctness (must work every time)
1) Build status semantics (done)
   - Add explicit leased state handling in the UI (do not show as building
     until the worker posts building).
   - Ensure control-plane updates building only after runner start.
   - Success: a full queue shows pending/leased/building accurately.

2) Queue pop sizing (done)
   - Cap build queue pop to build pool size (or introduce a separate leased
     state that is not rendered as running).
   - Success: no more than BUILD_POOL_SIZE jobs show as building.

3) Lease timeout + requeue safety (done)
   - Add lease expiration and auto-requeue on worker crash.
   - Success: stalled builds return to pending after timeout.

4) Plan -> build linkage integrity (done)
   - Ensure plan_id and node_id always flow into build_status and events.
   - Success: every build row links back to its plan node and UI shows it.

5) Log streaming correctness (done)
   - Guarantee seq ordering, per-build chunk replay, and no gaps on reconnect.
   - Add UI indicator for live vs replay.
   - Success: a running build shows live logs; reconnect resumes cleanly.

6) End-to-end test script (done)
   - Add a script that uploads a simple requirements.txt, plans, builds,
     validates success, and verifies artifacts in CAS/object storage.
   - Success: a single command verifies the entire pipeline end-to-end.

## P1: Production readiness (durability, safety, operability)
### Reliability and resilience
1) Failure reason codes (done)
   - Standardize reason codes (missing header, missing module, linker error,
     toolchain missing, pkg-config missing, CMake failure, etc.).
   - Store in event metadata and show as chips in UI.
   - Success: every failed attempt has a structured reason code.

2) Auto-fix guardrails (done)
   - Add confidence thresholds, dedupe, and rate limits per package/version.
   - Flag high-impact recipes (e.g., build-essential) in UI.
   - Success: auto-fixes are safe, explainable, and not noisy.

3) Decision trace (done)
   - Emit a decision trace for why hints were applied/blocked.
   - Render a compact decision log in the package view.
   - Success: every retry has a visible, human-readable decision trail.

4) Build attempt timeline (done)
   - Compact timeline showing attempt -> recipes -> outcome.
   - Add before/after diff of recipes between attempts.
   - Success: a user can understand the build history at a glance.

### Artifact lifecycle + pip repo readiness
5) Wheelhouse index (PEP 503) (done)
   - Generate a simple index from object storage (MinIO).
   - Expose a read-only pip-compatible endpoint.
   - Success: pip can install from the wheelhouse without custom tooling.

6) Artifact manifests + immutability (done)
   - Ensure every artifact has a signed/immutable manifest (digest, policy,
     runtime/pack versions, toolchain hash).
   - Success: artifacts are auditable and reproducible.

7) Retention and cleanup (done)
   - Add retention rules for log chunks, build artifacts, and retry history.
   - Success: storage growth is bounded and predictable.

### Observability + operations
8) Metrics coverage (done)
   - Queue depth, build durations, failure rate, retry churn, worker heartbeat,
     log stream throughput, CAS hit rate.
   - Success: Prometheus dashboard shows live system health.

9) Alerts + runbooks (done)
   - Alert on queue backlog, build failure spikes, worker silence, DB errors.
   - Provide runbooks for recovery steps.
   - Success: on-call can resolve issues quickly.

10) Backup/restore (done)
    - Postgres, MinIO, and Zot backup strategy + restore verification.
    - Success: disaster recovery is documented and tested.

### Security + access control
11) Token scoping + rotation (done)
    - Separate tokens for worker actions vs UI actions; rotate regularly.
    - Success: least-privilege tokens with documented rotation steps.

12) Resource isolation (done)
    - Enforce worker CPU/memory limits, disk quota for cache.
    - Success: builds cannot starve the host or fill disks.

## P2: Above-and-beyond polish + scale
### UX refinement
1) Smooth UI refresh (done)
   - Reduce flicker in queue tables; maintain stable row ordering.
   - Success: tables update without full redraws.

2) Empty/blocked states (done)
   - More informative empty states with recommended actions and links.
   - Success: user always knows the next action.

3) Live status clarity
   - Add a build-progress banner ("queued", "leased", "building", "repairing").
   - Success: users can understand current state instantly.

4) Retry queue redesign
   - Continued declutter: separate controls from list, collapse advanced
     actions, and surface the highest-value actions first.
   - Success: visually minimal but feature-complete panel.

### Performance and scale
5) CAS/object storage batching
   - Batch uploads and downloads; parallelize safely with limits.
   - Success: faster builds under load without saturation.

6) Build dedupe + reuse
   - Detect duplicate queue entries and coalesce in-flight builds.
   - Success: no redundant builds when the same artifact is already running.

7) Worker sharding
   - Support multiple workers with clear ownership and safe leasing.
   - Success: horizontal scale without double builds.

### Docs + onboarding
8) Production deployment guide
   - A single definitive doc for production: infra, env vars, secrets,
     backup/restore, and SLOs.
   - Success: a new operator can deploy in one pass.

9) Architecture + data flow diagrams
   - Keep diagrams current with log streaming and auto-fix flows.
   - Success: diagrams align with actual system behavior.
