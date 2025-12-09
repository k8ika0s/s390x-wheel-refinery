The web UI is the refinery’s “control panel.” It’s a React single-page app that lets humans see what’s happening, filter events, manage the queue, peek at hints, and inspect per-package history without touching the command line. Think of it as the dashboard and remote control for the refinery.

**Purpose / Responsibilities**
- Show summaries (status counts, failures/slowest), recent events, and per-package detail (variants, failures, hints, logs).
- Manage queue actions (retry selected, clear, trigger worker) and display worker mode/length.
- Display hints, metrics, and logs with autoscroll/open/copy URL.
- Provide theme toggle, filters, pagination/sorting, toasts, and loading/empty states.

**Why it matters**
- Makes the system usable and observable for operators who aren’t living in the shell.
- Surfaces the data needed to decide next actions and troubleshoot builds.

**Fit in the system**
- Calls the API/control plane; mirrors history/queue state; runs as its own container.

**Current status**
- SPA with dashboard filters, queue table/bulk actions, package tabs (overview/events/hints), log viewer (autoscroll/open/copy URL), theme toggle, pagination/sorting, empty states, toasts.
- Tests (vitest) in place; Tailwind-based styling; served via dedicated UI container.

**Next steps / gaps**
- Hook into new backend endpoints: build plan/manifest view, hint editor, history search, cache/worker insights, log search/tail.
- Add richer sidebar/nav styling and optional light/dark illustrations for empty/loading states.
