# Plan/Build Queue Split — Implementation Plan

This document tracks the staged implementation to separate planning and building into distinct queues and worker pools.

## Goals
- Avoid requirements.txt overwrite races.
- Separate concerns: plan generation vs. build execution.
- Support auto/manual modes for both stages.
- Surface queue/worker status and actions in the UI.

## Target Architecture
- **Watcher**: monitors `/input`, waits for stable files, records them as pending inputs.
- **Plan queue**: Redis list `plan_queue` holds pending input IDs for planning.
- **Plan workers**: pop `plan_queue`, run planner, write plan artifacts, update DB.
- **Build queue**: Redis list `build_queue` holds plan IDs ready for builds.
- **Build workers**: pop `build_queue`, run builds (existing worker logic), emit events/logs/manifests.
- **Metadata (Postgres)**:
  - `pending_inputs`: id, filename, digest, size, status (pending|planning|planned|failed), created_at, updated_at, error.
  - `plans`: id, pending_input_id, status (ready_for_build|building|built|failed), summary JSON, plan_blob_url/digest, created_at, updated_at, error.
- **Settings**: add `auto_plan`, `auto_build`, and watcher dir to `/api/settings`.

## UX Expectations
- Show plan queue status: auto/manual toggle, counts, worker pool size.
- Show pending inputs (manual mode) with “Generate plan” action (enqueue to plan queue).
- Show plans list with status; in manual build mode, “Enqueue build” action (enqueue to build queue).
- Show build queue status, worker pool size, and logs/events as today.

## API Additions (control-plane)
- `GET /api/pending-inputs`: list pending inputs.
- `POST /api/pending-inputs/{id}/enqueue-plan`: enqueue a pending input to `plan_queue`.
- `GET /api/plans`: list plans + status/summary.
- `POST /api/plans/{id}/enqueue-build`: enqueue a plan to `build_queue`.
- Extend `/api/settings` with `auto_plan`, `auto_build`, `watch_dir`.

## Worker Changes
- **Plan worker mode** (new): pops `plan_queue`, reads referenced file, runs planner, stores plan (DB + CAS/object store or JSONB), updates plan status; if `auto_build` is true, enqueues plan_id to `build_queue`.
- **Build worker**: unchanged core logic; input is plan_id popped from `build_queue`.
- **Watcher** (can be a tiny sidecar or part of control-plane): detects new stable files, writes `pending_inputs`, and either enqueues to `plan_queue` (auto) or leaves for manual action.

## Data Flow (auto)
1) Watcher sees new file → insert `pending_inputs` → enqueue to `plan_queue`.
2) Plan worker → generates plan → insert `plans` → enqueue plan_id to `build_queue` (if auto_build).
3) Build worker → executes jobs from plan → emits events/manifests/logs.

## Data Flow (manual plan/build)
1) Watcher inserts `pending_inputs` only.
2) User clicks “Generate plan” → enqueue to `plan_queue`.
3) Plan worker writes plan, marks status `ready_for_build`.
4) User clicks “Enqueue build” → push plan_id to `build_queue`.
5) Build worker executes.

## Incremental Steps
1) Schema: add `pending_inputs`, `plans` tables; migrate.
2) Settings: add `auto_plan`, `auto_build`, `watch_dir` to `/api/settings`.
3) Queues: add Redis keys `plan_queue`, `build_queue`; helpers.
4) APIs: pending-inputs and plans endpoints; enqueue actions.
5) Watcher: simple stability check then insert/enqueue (configurable auto/manual).
6) Plan worker mode: consume `plan_queue`, store plans, enqueue build when configured.
7) UI: toggles, lists (pending inputs, plans), actions (generate plan, enqueue build), status badges.
8) Polish: metrics per queue/worker pool, error surfaces, retries/backoff, pruning old pending/plan records.
