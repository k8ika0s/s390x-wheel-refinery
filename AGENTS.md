# AGENTS.md

This file is for Codex agents and human collaborators. Keep it updated when
workflow, architecture, or operational steps change. The goal is for any agent
to pick up work without losing context.

## Project overview
s390x Wheel Refinery is an end-to-end pipeline for building s390x-native Python
wheels. Inputs (requirements.txt or wheel files) are planned into a DAG and
built in a containerized worker, with automated fixes via hints/recipes, live
log streaming, and artifact publishing to CAS (Zot) and object storage (MinIO).

## Architecture (current)
- Control-plane (Go): API + Postgres store for plans, pending inputs, build
  status, events, hints, logs, and log chunks.
- Worker (Go): polls plan/build queues, runs builds via Podman inside the
  builder image, streams logs live, and posts status/events/logs.
- Builder image: contains toolchains and recipes for packs/runtimes/repair.
- UI (React/Vite): Inputs, Plans, Builds, Hints, Settings, log viewer, and DAG
  graph visualization.
- Storage: Zot for CAS (packs/runtimes/wheels/repairs), MinIO for inputs and
  output wheelhouse, local /cache for staging/pip cache.

## Repo map
- go-control-plane/: API + store + migrations + WebSocket log hub
- go-worker/: queue loops + runner + auto-fix + log streaming sender
- ui/: React UI
- recipes/: pack/runtime/repair scripts
- containers/: Containerfiles for builder/control-plane/worker/ui
- docs/: user guide, automation details, queue model, diagrams
- podman-compose.yml: local/remote stack
- scripts/: seed and helper scripts

## Common commands (local)
- Start stack: podman compose -f podman-compose.yml up
- Build builder image: podman build -f containers/refinery-builder/Containerfile -t refinery-builder:latest .
- Build all s390x service images with host networking: ./scripts/build-stack-images-hostnet.sh
- Start from prebuilt images only: ./scripts/stack-up-no-build.sh
- Capture compose status + recent logs + health probes: ./scripts/stack-diagnostics.sh
- Tests:
  - go-control-plane: (cd go-control-plane && go test ./...)
  - go-worker: (cd go-worker && go test ./...)
  - UI: (cd ui && npm test)
- UI dev:
  - VITE_API_BASE=http://localhost:8080 npm run dev -- --port 3005

## Remote workflow (zkd0)
- SSH alias: zkd0
- Tmux session: kd1 (always use tmux; use mcp-tmux tools for remote commands)
- Remote root: ~/s390x-wheel-refinery (rsync-only; do not use git on host)
- Build + run (no-cache when requested):
  1) make prep-dirs
  2) `podman build --network host --no-cache -f containers/refinery-builder/Containerfile -t refinery-builder:latest .`
  3) `podman build --network host --no-cache -f containers/go-control-plane/Containerfile -t localhost/s390x-wheel-refinery_control-plane:latest .`
  4) `podman build --network host --no-cache -f containers/go-worker/Containerfile -t localhost/s390x-wheel-refinery_worker:latest .`
  5) `podman build --network host --no-cache -f containers/ui/Containerfile -t localhost/s390x-wheel-refinery_ui:latest .`
  6) `podman build --network host --no-cache -f containers/zot/Containerfile -t localhost/s390x-wheel-refinery_zot:latest .`
  7) `podman build --network host --no-cache -f containers/minio/Containerfile -t localhost/s390x-wheel-refinery_minio:latest .`
  8) podman compose down --remove-orphans
  9) podman compose up -d --force-recreate --no-build
  10) podman compose ps

## Operational notes / gotchas
- /cache is required; make prep-dirs creates cache/cas/pip/plans.
- Worker container uses Podman and must be privileged.
- Recipes must be available in the worker container (/app/recipes). Compose
  mounts ./recipes to /app/recipes.
- Service container images are built for `linux/s390x`. On non-s390x developer
  machines, local `podman compose up` can build images but the containers will
  fail at runtime with `Exec format error`; use local tests/builds for fast
  feedback and `zkd0` for full-stack runtime validation.
- Example first-run requirement sets live under `examples/requirements/`.
- `scripts/upload-requirements.sh` uploads a full requirements file and enqueues
  planning; `scripts/seed-build.sh` remains the single-package smoke runner.
- Build status uses leased vs building; UI should reflect this distinction.
- Auto-build requires both control-plane AUTO_BUILD and worker AUTO_BUILD.
- OpenAI-compatible inference uses worker-only `INFER_URL`/`INFER_TOKEN`
  secrets; prompts and retry/model tuning are controlled through `/api/settings`.
- `zkd0` currently needs direct host-networked `podman build --network host`
  per image. `podman compose build` still hits netavark bridge failures on some
  second-stage image steps there.
- Control-plane request logging is now live on the HTTP path and emits
  structured logs with `X-Correlation-ID` / `X-Request-ID` response headers.
- UI uses cache-busting index + immutable assets; hard refresh should update.

## Collaboration expectations
- Commit after each change chunk with clear message (user preference).
- Use mcp-tmux for remote actions and keep work visible in kd1.
- Avoid destructive git commands unless explicitly requested.
- Update CONTEXT.md when architecture/workflow or key decisions change.

## Primary docs to read first
- README.md (system overview)
- docs/user-guide.md (ops flow)
- docs/automatic-build-repair-system.md (automation + logs)
- docs/plan-build-queues.md (queue model)
- docs/diagrams/ (Mermaid diagrams)
