# Artifact-Centric CAS Plan (s390x Wheel Refinery)

## Decisions (recap)
- **Artifact store:** Zot for OCI/CAS artifacts (runtimes, packs, wheels, repairs); MinIO for wheel/object storage if desired.
- **CAS:** Content-address all artifacts (runtime, packs, wheels, repairs).
- **Runtimes:** On-demand CPython builds with a cache; add a prebuild script/job to populate common versions.
- **Packs:** Bundled artifacts with recipes; planner selects via a catalog.
- **DAG:** Runtime → packs → wheel → repair (implemented iteratively).
- **Builder:** Keep Python builder/refinery (PEP 517) with a wrapper for interpreter selection.
- **Queue/jobs:** Carry artifact digests (runtime, packs, source) from the planner.
- **Metrics:** Enrich metadata now; Prometheus later.

## Artifact model (keys)
- **Runtime (CPython):** `(arch, policy_base_digest, python_version, build_flags, toolchain_id, deps_hash) → digest`.
- **Pack:** `(arch, policy_base_digest, pack_name, pack_version(s), recipe_digest) → digest`.
- **Wheel:** `(source_digest, py_tag, platform_tag, runtime_digest, pack_digests[], build_frontend_version, config_flags_digest) → digest`.
- **Repair:** `(input_wheel_digest, repair_tool_version, policy_rules_digest) → digest`.

## Storage mapping
- **Zot (OCI/ORAS):** runtimes, packs, repairs, optionally wheels.
- **MinIO (optional):** wheel/object mirror.

### Local dev (compose)
- `docker-compose.control-plane.yml` now brings up Zot on `http://localhost:5000` (repo default: `artifacts`) and MinIO on `http://localhost:9000` (console `:9001`) with credentials `minio/minio123`.

## Planner / DAG
- **BuildRequest schema:** targets (sources, python_versions, policy, constraints, allowed packs), strategy, overrides.
- **Node types:** runtime, pack, wheel, repair with deps and digests; planner marks cache hits (lookup in Zot) vs builds.
- **Pack catalog:** YAML/TOML mapping package pattern/backend → pack names; pack definitions reference recipes and digests.

## Build pipelines
- **CPython runtime:** script/CI job builds versions on s390x base (policy image), pushes to Zot with labels/digest.
- **Packs:** recipes produce `/opt/packs/<name>/<digest>`; CI builds and pushes to Zot; catalog updated with digests.
- **Wheels/repairs:** built via Python builder; publish artifacts to Zot (and MinIO mirror if desired) with provenance.

## Worker / runner
- **Jobs:** include runtime_digest, pack_digests[], python_version/tag, source_digest.
- **Fetch/mount:** worker downloads artifacts to local CAS; bind-mount runtime/packs into build container; set PATH/LD_LIBRARY_PATH/PKG_CONFIG_PATH.
- **Wrapper:** builder image includes script to select interpreter based on PYTHON_VERSION/tag and invoke `refinery` CLI.

## Control-plane / metadata
- Store plan nodes with digests; enrich manifest/events/logs with python_version/tag, runtime/pack digests, base image digest, source/wheel digests. Emit “facts” per artifact (provenance, inputs/outputs, log refs).
- API: plan endpoints return DAG; queue enqueue accepts artifact IDs; artifact endpoints link Zot/MinIO locations.

## Metrics (v1)
- Add durations/counts and provenance fields; Prometheus later.

## Phased delivery (high-level)
1. Define schemas (BuildRequest, DAG nodes, artifact key structs).
2. Integrate CAS interfaces to Zot; configure Zot in compose.
3. Add CPython build/publish script and pack catalog + build CI.
4. Implement worker fetch/mount + wrapper for interpreter selection.
5. Enrich metadata and facts; publish wheels/repairs to Zot/MinIO.
6. Update UI/docs to surface digests/artifact links; add basic metrics.
