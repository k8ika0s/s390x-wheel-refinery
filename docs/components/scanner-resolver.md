The scanner and resolver are the refinery’s “eyes and brain” for understanding the wheels you upload. In plain terms: it reads the wheel files like a librarian scanning barcodes, figures out which ones are safe to reuse and which ones are platform-specific, expands their dependencies, and produces a clear list of what actually needs to be rebuilt for s390x. This up-front triage keeps us from rebuilding the world and sets the stage for a predictable, pinned build plan.

**Purpose / Responsibilities**
- Parse input wheels, classify pure vs native, and extract metadata (name, version, python/platform tags).
- Expand dependencies and decide reuse vs rebuild for the target Python/platform.
- Emit the internal build plan/graph the builder/worker will execute.

**Why it matters**
- Avoids unnecessary builds, speeds runs, and makes the resulting manifest accurate.
- Provides the authoritative “what and why” list for downstream components (builder, queue, UI).

**Fit in the system**
- Runs first when the refinery starts; hands a build plan and status counts to the builder/worker and to the UI/API for visibility.

**Current status**
- Implemented in code (scanner/resolver) and exercised by tests and the smoke run.
- Produces recent/failures data used by the API/UI dashboards.

**Next steps / gaps**
- Expose the build plan/graph via an API endpoint for UI consumption.
- Add richer reasoning in responses (e.g., “rebuilt because platform mismatch”).
- Optionally support multiple Python targets in one pass in the future.
