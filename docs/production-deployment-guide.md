# Production deployment guide

This guide is meant to help you deploy the refinery in a way that is reliable, observable, and easy to operate. It focuses on the
"why" behind each choice so you can adapt it to your environment rather than follow a rigid script.

## What "production" means here

The refinery is a multi-service system. The control-plane stores history and serves the UI, the worker executes builds, and the
builder image performs the heavy lifting. In production, the goal is not only to run the services, but to ensure you can reproduce
builds, recover after failure, and scale without data loss.

In practice that means:
- persistent storage for Postgres, CAS, and object storage,
- stable tokens for write actions,
- a predictable worker identity for auditing and sharding,
- and clear operational signals (logs, health checks, and metrics).

## Architecture at runtime

At a high level:
- The UI talks to the control-plane.
- The control-plane holds history, logs, and queue state in Postgres.
- The worker pulls build jobs from the control-plane, streams logs, and pushes artifacts to CAS and object storage.
- CAS is a content-addressed registry (Zot). Object storage holds inputs and wheel outputs.

The design is intentionally modular so you can swap in managed Postgres, Redis, or an existing S3-compatible store.

## Prerequisites and why they matter

### Host resources
- CPU and RAM: Builds are real compilation workloads. If the host is small, builds will queue up and appear to "hang."
- Disk: /cache and /output should live on a fast disk. The cache is used for CAS staging and pip downloads; slow disks make builds
  feel flaky even when they are not.
- Network: Ensure the worker can reach CAS and object storage reliably. Most build stalls are caused by intermittent storage access.

### Required services
- Postgres: Stores event history, logs, manifests, and queue state. The system assumes this data is durable and queryable.
- CAS registry (Zot): Provides content-addressed artifacts for packs, runtimes, wheels, and repairs. This is what lets the planner
  mark nodes as reuse vs build.
- Object storage (MinIO or S3 compatible): Stores uploaded inputs and a wheelhouse. This is where you build a future pip repo.
- Redis or Kafka (optional): Queue backends for scale. File queue is fine for single-node deployments but not for shared workers.

## Secrets and authentication

Set tokens even in small deployments so your UI write actions and worker posts are scoped:
- `UI_TOKEN` for UI-triggered actions.
- `WORKER_TOKEN` for worker posts (status, logs, events).

These tokens protect the operational surfaces that mutate state. See `docs/refinery-secrets.md` for the full list of secrets and
where they are used, plus `docs/token-rotation.md` for rotation guidance.

## Storage and volume layout

Think of storage in three tiers:
1) **Database**: Postgres volumes for history, logs, hints, manifests, and queue state.
2) **Shared object stores**: CAS and object storage for long-lived artifacts.
3) **Local worker cache**: `/cache` is the worker's scratch space for CAS staging and pip cache.

The worker needs `/cache` and `/output` to exist and be writable. They are local on the worker host and can be cleaned safely as
long as CAS and object storage are intact.

## Image build strategy

The system uses three main images:
- `refinery-builder:latest` (build tools and recipes).
- `s390x-wheel-refinery_control-plane` (API + UI proxy).
- `s390x-wheel-refinery_worker` (worker runtime).

In production, you generally want a consistent builder image across workers so builds are reproducible. Rebuild the builder image
only when you intend to change build tooling. That is the main reason the builder is separated from the control-plane.

## Configuration choices that affect reliability

### Worker identity
Set `WORKER_ID` to a stable value per worker. This gives you clear ownership in build status and avoids confusion when multiple
workers are active.

### CAS and object store concurrency
`CAS_MAX_PARALLEL` and `OBJECT_STORE_MAX_PARALLEL` cap concurrent transfers. These defaults protect your storage from bursts
without throttling the build loop. If you have fast storage, raising them can shorten build time.

### Build pooling
`BUILD_POOL_SIZE` controls how many builds run at once. This is the main lever for throughput. If it is too high relative to host
resources, you will see timeouts and "killed" builds.

## Startup and validation flow

A deployment is considered healthy when:
- `/health` and `/ready` return success on the control-plane.
- The UI shows a connected API status.
- Worker heartbeats show online in the Settings → Workers list.
- A test build produces logs and a manifest entry.

If any of those steps fail, the fastest path is to check control-plane logs first (API errors), then worker logs (build errors),
then storage logs (CAS or object store failures). The `docs/alerts-runbooks.md` guide maps common symptoms to likely causes.

## Scaling and multi-worker behavior

Multiple workers are supported, and build leases are stamped with `worker_id` so you can see which worker owns a job. This makes
it safe to scale horizontally without double builds. If you use a shared queue backend (Redis/Kafka), ensure all workers point to
the same control-plane and queue, and keep `WORKER_ID` unique per worker.

## Backups and retention

Production readiness depends on being able to restore data, not only re-run builds. Follow `docs/backup-restore.md` and make sure:
- Postgres is backed up regularly.
- Object storage buckets are retained (inputs and wheelhouse).
- CAS registry is backed up or mirrored if it is the source of truth.

Log retention settings exist to keep DB size reasonable. These do not delete CAS or object storage artifacts, only history.

## Security and isolation

The worker runs privileged with embedded Podman. That is a deliberate choice so builds can happen in clean containers, but it means
the worker host should be trusted and isolated from unrelated services. A dedicated host or VM is strongly recommended.

## When things go wrong

Most issues are one of three categories:
- **Connectivity**: worker cannot reach CAS or object store.
- **Capacity**: CPU, RAM, or disk pressure causes builds to stall or fail.
- **Auth**: tokens are missing or mismatched, so worker writes are rejected.

The runbooks in `docs/alerts-runbooks.md` explain how to validate each category without guesswork.
