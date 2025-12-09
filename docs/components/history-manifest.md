History and manifest are the refinery’s “memory.” They record every build/reuse event and produce the manifest that tells you exactly what came out of a run. For a non-technical reader: this is the ledger that says what we tried, what succeeded or failed, how long it took, and what wheels you can now use.

**Purpose / Responsibilities**
- Persist build events (status, timestamps, metadata like duration/variant/log path).
- Provide summaries for dashboards (status counts, recent failures/slowest).
- Emit a manifest describing resulting wheels in `/output` for each run.

**Why it matters**
- Traceability: know what was built, reused, or failed.
- Inputs to UI, metrics, and cache decisions.

**Fit in the system**
- Written by builder/worker and read by API/UI.
- Manifest is the final “bill of materials” for consumers of the wheels.

**Current status**
- BuildHistory in place; manifest writing on runs; surfaced via API summary/recent/failures.
- UI shows recent events/failures/variants using history.

**Next steps / gaps**
- Expose manifest download/view endpoint.
- Add richer history browsing/search (by package/status/date/run id) via API.
- Include cache hit/miss and variant info in history entries.
