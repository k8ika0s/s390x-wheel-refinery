# s390x Wheel Refinery — Current Architecture

## Components
- **Planner (Go)**: Scans `/input` wheels or `requirements.txt`, resolves pins, and emits a DAG: runtime → packs → wheels → repair. Checks CAS (Zot) for existing digests to mark nodes as `reuse` vs `build`.
- **Worker (Go)**: Drains the queue, downloads missing artifacts from CAS, extracts packs/runtimes, sets `DEPS_PREFIXES`, and runs builds in a builder container via Podman. Pushes wheels/repairs (and optional packs/runtimes) back to CAS/MinIO, and reports logs/manifest/events to the control-plane.
- **Builder image**: `refinery-builder:latest` (Containerfile in `containers/refinery-builder/`) with toolchains, auditwheel/patchelf, and `recipes/` mounted at `/app/recipes`.
- **Recipes**: Shell scripts for packs (openssl, zlib, libffi, sqlite, libxml2/xslt, libpng/jpeg/freetype, openblas, etc.), CPython runtimes, and auditwheel repair. Checksums pinned in `recipes/versions.sh`.
- **Control-plane (Go)**: API for history/manifest/queue/logs/hints/artifacts; metrics and Prometheus endpoint; forwards worker trigger; stores manifests/logs/events.
- **UI (React)**: Dashboard for events/status counts, metrics, artifacts, queue controls, and log viewing.
- **CAS/Object stores**: Zot for CAS blobs (packs/runtimes/wheels/repairs), MinIO optional for wheel mirroring.

## Build Flow
1) Planner produces a DAG with dependency edges (packs depend on pack deps; wheels depend on runtime+packs; repair depends on wheel). Nodes marked `build` if CAS miss.
2) Queue seeds work (file/Redis/Kafka). Worker pops jobs, fetches CAS artifacts, extracts them, and mounts pack/runtimes into the builder container.
3) Runner sets env (`DEPS_PREFIXES`, `RUNTIME_PATH`, digests, etc.) and runs the default build command inside `refinery-builder`.
4) Repair step runs auditwheel (policy `manylinux2014_s390x` by default) to emit `<name>-<version>-repair.whl`.
5) Worker uploads artifacts to CAS/MinIO, posts logs/events/manifest to control-plane, and updates UI via API.

## Defaults and Env
- `CONTAINER_IMAGE=refinery-builder:latest`, `PACK_RECIPES_DIR=/app/recipes`, `DEFAULT_RUNTIME_CMD=/app/recipes/cpython311.sh`, `DEFAULT_REPAIR_CMD=/app/recipes/repair.sh`.
- CAS: `CAS_REGISTRY_URL/REPO`; Local cache: `LOCAL_CAS_DIR`.
- Repair: `REPAIR_CMD` optional; default uses auditwheel. Tool/policy metadata: `REPAIR_TOOL_VERSION`, `REPAIR_POLICY_HASH`.
- Pack builds honor DAG ordering; `DEPS_PREFIXES` passed to recipes; packs/runtimes are extracted before mount.

## Known TODOs
- Ensure worker runs with the builder image in all deployments.
- Enforce non-stub outputs (fail manifest-only) once real recipes are active.
- Continue evolving pack dependency metadata in the catalog instead of hardcoded map.
