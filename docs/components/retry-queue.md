The retry queue is the refinery’s “to-do list” for rebuild requests. It’s a simple file-backed queue that holds which packages to retry, with what versions and recipes. For non-technical readers: when something needs another attempt, it gets dropped into this list so the worker can pick it up later.

**Purpose / Responsibilities**
- Persist retry requests (package, version, python/platform tags, recipes).
- Support enqueue, list, clear, and pop-all operations.
- Allow worker to process queued items on demand or via webhook trigger.

**Why it matters**
- Decouples planning from execution; retries aren’t lost between runs.
- Enables manual “run worker now” or automated draining of pending items.

**Fit in the system**
- Enqueued by API/UI and the builder (for dependent rebuilds).
- Consumed by worker/`process_queue`; state surfaced in UI.

**Current status**
- Queue implemented with list/clear APIs; UI shows items and supports bulk retry/clear.
- Worker trigger endpoint processes the queue (local or webhook mode).

**Next steps / gaps**
- Add age/sorting in API responses for better UI display (oldest first).
- Optional: expose queue stats (oldest age, total attempts) for dashboards.
