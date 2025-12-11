# Go Resolver (Design Draft)

## Layperson context
Today the Go worker still shells out to the Python CLI to generate `plan.json` when it is missing. The goal of the Go resolver is to drop that dependency and let the Go stack (control-plane + worker) scan the input wheels, decide what can be reused, and plan exactly which packages must be rebuilt for s390x. This document describes what the resolver must do, how it fits into the system, and the work needed to implement and integrate it.

## Responsibilities
- **Scan input wheels:** Parse wheel filenames/metadata from `/input`, classify pure/compatible wheels vs platform-specific wheels for the target Python tag and platform tag.
- **Reuse vs rebuild:** Mark pure/compatible wheels as reusable; mark others for rebuild with name, version, python tag, and platform tag.
- **Dependency expansion:** Consult package metadata (requires_dist) and the configured indexes to plan missing dependencies when `fallback_latest` or dependency expansion is enabled, with depth/breadth limits to avoid explosion.
- **Upgrade strategy:** Support at least the pinned strategy (respect implied versions from input wheels); keep room to add eager/latest-compatible later.
- **Indexes/config:** Respect index settings (primary/extra indexes, trusted hosts) and per-package overrides/hints as provided by the control-plane config.
- **Output:** Write `plan.json` in the same shape the Python CLI produces so the worker and UI can consume it unchanged (reuse/to_build/dependency_expansions/missing_requirements plus metadata like python_tag, platform_tag, run_id).
- **Error handling:** Produce clear errors for missing pins, incompatible inputs, or index lookup failures; no partial/ambiguous plans.

## Integration points
- **Control-plane API:** Expose a resolver endpoint so the UI/API can trigger a plan and store it alongside history/manifest.
- **Worker:** Replace the Python CLI fallback in the worker with the Go resolver; prefer `/output/plan.json`, then `/cache/plan.json`, otherwise call the Go resolver.
- **History/hints:** Feed the plan builder with known failures/hints to allow skip-known-failures and hint-driven retries to stay consistent with the Python flow.

## Success criteria
- Running the Go resolver against a given `/input`, `/cache`, and target python/platform yields a `plan.json` identical (or materially equivalent) to the Python CLI for the same inputs/config.
- Worker no longer shells out to Python; control-plane can serve plans to the UI and worker.
- Tests cover pure/compatible reuse, platform rebuilds, dependency expansion (bounded), and failure modes (missing pins, incompatible wheels).

## Work plan (proposed)
- Implement wheel scanner + metadata parser in Go (filename tags + METADATA parsing for requires_dist). ✅ METADATA Requires-Dist parsed to add dependency nodes (default version “latest”).
- Implement plan builder (reuse vs rebuild, pinned strategy, optional dependency expansion with limits).
- Implement index client in Go (respect index/extra index and trusted hosts); support source lookups for dependency expansion.
- Define plan JSON schema matching the Python `plan_snapshot`.
- Wire control-plane endpoint for “compute plan” and store it to cache/output.
- Update worker to call the Go resolver (drop Python fallback).
- Add parity tests vs recorded Python plans for sample fixtures; add unit tests for scanner, planner, and upgrade strategy. ✅ Fixture-based regression added.
- Add integration fixture under `go-worker/internal/plan/testdata/` with representative wheel names and expected plan.json for regression testing. ✅ Added.
- Wire control-plane `/api/plan/compute` to proxy plan generation to the worker (`WORKER_PLAN_URL` / `/plan`), so UI/API can trigger plan generation. ✅ Added.
