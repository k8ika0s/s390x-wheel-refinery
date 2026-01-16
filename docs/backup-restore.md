# Backup and restore

This document defines the backup and restore procedure for Postgres, MinIO, and
Zot. The goal is to support disaster recovery and routine verification.

## What gets backed up

- Postgres (control-plane state, events, build attempts, logs metadata)
- MinIO (inputs + wheelhouse artifacts)
- Zot (CAS artifacts: packs, runtimes, wheels, repairs)

## Backup procedure

1) Run the backup script from the repo root:

```
./scripts/backup-stack.sh
```

2) The script creates `backups/<timestamp>/` containing:
   - `postgres.sql` (logical dump)
   - `pgdata.tar.gz` (volume snapshot)
   - `miniodata.tar.gz` (volume snapshot)
   - `zotdata.tar.gz` (volume snapshot)
   - `manifest.txt`

3) Store the backup directory in your durable storage location.

## Restore procedure

1) Pick the backup directory to restore (example below):

```
./scripts/restore-stack.sh backups/20240321-153000
```

2) The script will:
   - Stop the stack
   - Restore volume archives (pgdata, miniodata, zotdata)
   - Start the stack
   - Apply `postgres.sql` if present

If you prefer a pure logical restore, remove `pgdata.tar.gz` and keep
`postgres.sql` before running the script.

## Verification checklist

After restore, verify:

- UI loads and `/api/metrics` returns 200.
- Postgres is responding: `podman exec <postgres> psql -U refinery -d refinery -c "select count(*) from build_status;"`
- MinIO has the wheelhouse bucket and objects (use MinIO console or `mc ls`).
- Zot registry responds: `curl -fsS http://localhost:5000/v2/_catalog`.
- A sample plan and build can be enqueued and completed.

## Notes

- The volume archives are full snapshots. For large datasets, use object store
  lifecycle policies to offload older content and reduce backup size.
- Keep `METRICS_WINDOW_MINUTES` aligned with alert thresholds in
  `docs/alerts-prometheus.yml`.
