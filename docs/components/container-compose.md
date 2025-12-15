Containers and compose files are the refinery’s “packaging and wiring.” They define how the API/worker and UI run in isolated environments, which volumes are mounted, and how services talk to each other. For a layperson: this is the recipe that spins up the whole system with the right folders shared for input, output, and cache.

**Purpose / Responsibilities**
- Build/run the Go control-plane + Go worker containers and the UI container (static SPA served on 3000).
- Wire volumes `/input`, `/output`, `/cache`, and pass worker token/webhook/env via compose.
- Allow local development (`npm run dev` for UI, Go binaries for control-plane/worker), and production builds via Containerfiles.

**Why it matters**
- Ensures repeatable, host-safe execution (no host pollution) and easy bring-up on s390x hosts.
- Simplifies deployment with a single `docker-compose up`.

**Fit in the system**
- Defines runtime topology: API + optional worker + UI services.
- Provides environment variables for worker mode, tokens, API base, mounts.

**Current status**
- Containerfiles for Go control-plane, Go worker, and UI exist; podman-compose includes control-plane, worker, UI, Postgres, Redis, and Redpanda. Worker runs privileged to allow embedded podman.
- CORS configured for SPA; mounts placeholders for input/output/cache.

**Next steps / gaps**
- Validate compose on target s390x host; document required env vars and volume permissions.
- Add optional external worker service/webhook example and smoke-test command.
- Consider multi-arch build artifacts for the UI image if needed.
