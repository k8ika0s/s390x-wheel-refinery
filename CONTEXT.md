# CONTEXT.md

This file captures the current technical and operational context. Update it
whenever major workflows, data models, or architecture change.

Last updated: 2025-01-28 (approx)

## Current focus
The Go control-plane + Go worker stack is the primary pipeline. The UI and
control-plane are wired for live log streaming, hint/recipe automation, and
per-plan build controls. Remote deploys happen via rsync to zkd0 and Podman
compose rebuilds in tmux session kd1.

## Current system behavior
- Inputs (requirements or wheels) are uploaded to object storage and tracked
  in pending_inputs.
- Planning generates a DAG and stores it in plans + plan_metadata.
- Builds are stored in build_status and are drained by the worker auto-poll
  loop (BUILD_POLL_INTERVAL_SEC).
- Worker leases jobs (status=leased), then posts building once the container
  starts.
- Auto-fix applies hints/recipes and retries when configured.
- Logs stream live from worker -> control-plane -> UI (NDJSON chunks + WS).
- Artifacts are stored in CAS (Zot) and optionally mirrored to object storage.

## Key data tables (Postgres)
- pending_inputs: uploaded input metadata + status.
- plans: plan JSON and optional DAG.
- plan_metadata: link between pending_inputs and plans.
- build_status: durable build queue with attempts/recipes/hints.
- events: build/plan history and automation metadata.
- logs: summarized logs per package/version.
- log_chunks: streaming log chunks with seq/timestamp.
- hints: catalog for auto-fix matching and inferred hints.

## Queue/status semantics
- Pending inputs: pending -> planning -> planned -> queued/build_queued -> done
- Build status: pending -> leased -> building -> built/failed/retry
- See docs/plan-build-queues.md for the canonical model.

## Log streaming design
- Worker posts NDJSON chunks to POST /api/logs/stream/{name}/{version}.
- Control-plane stores chunks in log_chunks and broadcasts on WebSocket.
- UI loads existing chunks then tails the WebSocket for live updates.

## Configuration highlights
- Control-plane: AUTO_PLAN, AUTO_BUILD, UI_TOKEN, WORKER_TOKEN,
  METRICS_WINDOW_MINUTES, CAS_REGISTRY_*, OBJECT_STORE_*.
- Worker: AUTO_BUILD, BUILD_POLL_INTERVAL_SEC, BUILD_POOL_SIZE, PLAN_POLL_*,
  CACHE_MAX_BYTES, CACHE_PRUNE_INTERVAL_SEC, CONTAINER_IMAGE, PACK_RECIPES_DIR,
  DEFAULT_RUNTIME_CMD, DEFAULT_REPAIR_CMD, CAS_REGISTRY_*, OBJECT_STORE_*,
  INFER_URL, INFER_TOKEN, INFER_MODEL, INFER_TIMEOUT_SEC, INFER_MAX_RETRIES.
- Compose limits: WORKER_CPU_LIMIT, WORKER_MEM_LIMIT (worker container caps).

## Remote deployment (zkd0)
- SSH alias: zkd0
- tmux session: kd1 (required)
- Root: ~/s390x-wheel-refinery (rsync only; no git on host)
- Rebuild pattern: build each image explicitly with `podman build --network
  host`, then `podman compose up -d --force-recreate --no-build`.
- `zkd0` Podman builds need host networking, and `podman compose build` is not
  sufficient there because some second-stage image steps still hit netavark
  bridge failures.
- Local Macs can build the compose images but cannot run the full stack because
  the service containers are built for `linux/s390x`; runtime validation needs
  an s390x host such as `zkd0`.
- Worker inference secrets stay in the environment (`INFER_URL`, `INFER_TOKEN`);
  prompt text and retry/model policy come from control-plane settings.

## Recent UI behaviors
- Plans panel includes Builds, Hints, Recipes, Graph, and Non-builds tabs.
- Plan graph is opened via a button and filters to a focused subtree when
  a node is clicked (no zoom transform).
- Build logs are live and persisted; package view shows status/time-in-state.

## Known gaps / open items
- Tighten leased vs building semantics in UI (avoid marking all leased items
  as building).
- P2 polish + scale items remain (retry queue redesign, batching, dedupe,
  sharding) — see prod-march-todo.md.

## Legacy notes
- .codex-context and .codex-context-catchup contain historical notes; they may
  be stale. Use this file + current docs as the source of truth.
