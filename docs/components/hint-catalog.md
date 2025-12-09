The hint catalog is the refinery’s “recipe book” for tricky builds. In simple terms: when a build fails because a system library is missing, hints map error patterns to the packages or commands that usually fix it. This lets the system learn and retry smarter instead of getting stuck.

**Purpose / Responsibilities**
- Store patterns and recommended system packages/recipes (dnf/apt/etc).
- Attach hints to failures and feed recipes into retries automatically.
- Provide visibility in the UI so users can see which hints applied.

**Why it matters**
- Reduces manual intervention for common build failures.
- Captures tribal knowledge so future runs benefit automatically.

**Fit in the system**
- Read by builder/worker when retrying; surfaced by API/UI for visibility.
- Should become editable so users can evolve recipes over time.

**Current status**
- Read-only hint catalog loaded from YAML; hints shown in UI (dashboard and package detail).
- Hints are applied to retries when recipes are present.

**Next steps / gaps**
- Add API endpoints for CRUD on hints (add/edit/delete).
- Track which hint matched which build in history for auditing.
- Provide UI editor once write endpoints exist.
