The builder/worker is the “hands” of the refinery: it takes each job from the plan, grabs sources, installs build dependencies, runs the build, and drops s390x wheels into cache and output. Think of it as a repeatable assembly line that can run locally or inside a dedicated worker container, retrying with different variants when builds are finicky.

**Purpose / Responsibilities**
- Given a job (name, version, python tag, platform), fetch sdist/source, install build deps, run PEP 517 build, and store wheels to `/cache` and `/output`.
- Apply hint-derived system recipes and retry strategies (variants/no-isolation/arch tweaks).
- Record logs/events for observability and cache hits vs rebuilds.

**Why it matters**
- Produces the actual s390x wheels that satisfy the plan.
- Encodes resilience (retries, hints) to keep builds moving despite missing system deps.

**Fit in the system**
- Consumes the resolver’s plan and the retry queue; writes to history/manifest.
- Can run inline (API) or via webhook/worker container for isolation.

**Current status**
- Implemented builder with retries/variants and hint application.
- Worker trigger endpoint exists; local and webhook modes supported; queue processed via `process_queue`.

**Next steps / gaps**
- Add build-plan-aware logging (which variant worked) and surface that via API/UI.
- Add optional “smoke test” endpoint to validate worker mounts/config.
- Future: per-job containerization for stronger isolation.
