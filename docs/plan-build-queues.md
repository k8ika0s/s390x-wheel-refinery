# Plan and Build Queues — Current Model

This document describes the **current** queue model used by the control-plane and worker. It replaces the earlier implementation plan and reflects the system as it exists today.

## Goals
- Separate planning from building to avoid input races.
- Keep the build queue durable and resumable.
- Support manual and automated entry points.
- Surface status and ownership clearly in the UI.

## Current Architecture
### Inputs
- Uploads land in object storage (MinIO/S3) and become **pending inputs**.
- The control-plane stores each pending input in `pending_inputs` with metadata.

### Plan queue (pending inputs)
- The plan queue holds **pending input IDs** for planning.
- Worker plan loops pop from the queue using:
  - `POST /api/pending-inputs/pop?max=N`
- The control-plane updates the pending input status to `planning`.
- The worker posts the resulting plan to:
  - `POST /api/plans`
- The control-plane stores the plan in the `plans` table and links it in `plan_metadata`.

### Build queue (build status table)
- Build jobs are stored in `build_status` rows.
- Enqueuing builds writes rows into `build_status` with `status=pending`.
- Workers claim jobs via:
  - `POST /api/build-queue/pop?max=N`
- The control-plane marks them `leased` and increments attempts.
- The worker then posts `building` when the container starts.

## Data Model (Postgres)
- `pending_inputs`: uploaded requirements/wheels awaiting planning.
- `plans`: plan snapshot + optional DAG JSON.
- `plan_metadata`: links `pending_inputs` to `plans` with a status.
- `build_status`: the durable build queue with attempts, backoff, and timestamps.

## Status Lifecycles
See `docs/diagrams/queue-status-lifecycle.mmd` for the full flow.

### Pending input statuses
| Status | Meaning |
| --- | --- |
| pending | Uploaded; waiting for plan enqueue. |
| planning | Enqueued to plan queue or currently being planned. |
| planned | Plan saved and linked. |
| queued | Builds enqueued for the linked plan. |
| build_queued | Alternate queued marker for UI visibility. |
| failed | Planning failed; requires retry. |

### Build statuses
| Status | Meaning |
| --- | --- |
| pending | Queued in `build_status` but not leased. |
| leased | Worker claimed the job; attempts incremented. |
| building | Runner started the container. |
| retry | Requeued after failure with recipes. |
| built | Successful build. |
| failed | Build failed and no retry queued. |
| cached | Output reused from cache. |
| reused | Output reused from previous build. |
| missing | Required input missing. |
| skipped_known_failure | Skipped due to known failure rule. |
| system_recipe_failed | System recipe failed before build. |

## Auto vs Manual Flow
### Auto-plan (control-plane)
- `AUTO_PLAN=true` enqueues pending inputs on upload.
- `AUTO_PLAN=false` leaves uploads in `pending` until manual enqueue.

### Auto-build (control-plane + worker)
- `AUTO_BUILD=true` in the **control-plane** enqueues builds immediately after plan save.
- `AUTO_BUILD=true` in the **worker** enables the build drain loop.
- If either side is off, builds may not start until manually enqueued/triggered.

## Worker Behavior
- **Plan polling**: Enabled with `PLAN_POLL_ENABLED=true`. The worker calls `/api/pending-inputs/pop` at `PLAN_POLL_INTERVAL_SEC`.
- **Build polling**: Enabled with `AUTO_BUILD=true`. The worker calls `/api/build-queue/pop` at `BUILD_POLL_INTERVAL_SEC`.
- **Concurrency**: `PLAN_POOL_SIZE` and `BUILD_POOL_SIZE` cap parallelism.
- **Settings overlay**: The worker periodically reads `/api/settings` to update pool sizes and python/platform tags.

## API Touchpoints
- Uploads:
  - `POST /api/requirements/upload`
  - `POST /api/wheels/upload`
- Planning:
  - `POST /api/pending-inputs/{id}/enqueue-plan`
  - `POST /api/pending-inputs/pop`
  - `POST /api/pending-inputs/status/{id}`
- Plans:
  - `GET /api/plans`
  - `GET /api/plans/{id}`
  - `POST /api/plans/{id}/enqueue-builds`
  - `POST /api/plans/{id}/enqueue-build`
- Builds:
  - `POST /api/build-queue/pop`
  - `POST /api/builds/status`
  - `GET /api/builds`

## UI Expectations
- **Inputs page** shows pending inputs with enqueue/retry actions.
- **Plans page** shows plan list and per-node build enqueue.
- **Builds page** shows build status, attempts, and log access.
- **Package view** shows the event timeline and log tailing.

## Known Gaps / Improvements
- UI `auto_plan` / `auto_build` toggles are informational; they do not override env-configured loops.
- Plan queue metrics are limited; expose queue depth for plan queue backend.
- Add integration tests for upload → plan → build end-to-end.
