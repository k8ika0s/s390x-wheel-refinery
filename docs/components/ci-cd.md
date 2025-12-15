CI/CD is the refinery’s “safety net.” It runs automated linting, tests, and smoke checks so changes don’t break the pipeline. In plain language: every change is put through quick checks to catch mistakes before they reach production.

**Purpose / Responsibilities**
- Run lint/tests for Go control-plane/worker, Python builder (ruff/pytest), UI (vitest), and smoke runs.
- Optionally run manual s390x build/test via QEMU or external runner.
- Gate PRs and provide fast feedback.

**Why it matters**
- Prevents regressions in scanner, builder, API, and UI.
- Keeps the UI and API in sync and ensures the CLI/refinery still runs end-to-end.

**Fit in the system**
- Executes on GitHub Actions with cache for pip; includes dummy smoke-run to exercise scan/plan/manifest.
- Supports a manual s390x job for platform validation.

**Current status**
- CI workflow with Go tests (control-plane/worker), Python lint/tests (ruff/pytest), UI tests (vitest), smoke-dummy-run; manual s390x job present.
- Recent fixes aligned UI with Go API; all jobs green on main.

**Next steps / gaps**
- Add new tests when build plan/manifest/hint CRUD endpoints land.
- Consider caching npm deps for UI job and adding log-search tests when implemented.
- Integrate optional QEMU/external runner for real s390x smoke if available.
