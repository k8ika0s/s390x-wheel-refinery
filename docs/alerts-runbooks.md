# Alerts and runbooks

This document defines the operational alerts for s390x Wheel Refinery and the
runbooks for responding. The alerts are expressed in Prometheus terms; see
`docs/alerts-prometheus.yml` for a ready-to-import rule group.

The metrics window for attempt and log throughput stats is controlled by
`METRICS_WINDOW_MINUTES` (default: 60). Adjust alert thresholds to match that
window.

## Alert catalog

### Build queue backlog

Signal
- `refinery_build_queue_length` stays above 0 and
  `refinery_build_queue_oldest_seconds` exceeds 900s (15m).

Impact
- Builds are stuck waiting; expected throughput is not happening.

Runbook
1) Check Build queue UI to confirm backlog and oldest age.
2) Verify workers are online (`refinery_workers_online`).
3) Confirm auto-build is enabled (Settings) and the worker is running.
4) Inspect worker logs for queue pop errors or token issues.
5) If queue backend is unhealthy, restart queue service or clear stuck leases.

### Build failure spike

Signal
- `refinery_build_failure_rate` > 0.30 for 10m.

Impact
- Many builds are failing; may indicate bad runtime/toolchain or systemic
  dependency break.

Runbook
1) Inspect Top failures and recent events for reason code clustering.
2) Check whether recent hints/recipes were auto-applied; look at decision traces.
3) If a known regression, quarantine the hint/recipe or roll back the builder.
4) Retry a small subset with manual overrides to validate fixes before resuming.

### Worker silence

Signal
- `refinery_workers_online == 0` for 120s.

Impact
- No builds will run; queue will stall.

Runbook
1) Verify worker container is up; restart if necessary.
2) Check `WORKER_TOKEN` configuration; ensure token is valid.
3) Confirm control-plane is reachable from the worker network.
4) Inspect heartbeat logs for errors.

### Database errors

Signal
- `refinery_db_up == 0` for 60s or sustained API 5xx responses.

Impact
- Plan/build state and logs may not be recorded; UI will degrade.

Runbook
1) Check Postgres container health and disk usage.
2) Review control-plane logs for SQL errors or migrations.
3) Ensure network connectivity between control-plane and DB.
4) Restore from backup if corruption is detected.

### Log stream stall

Signal
- `refinery_log_chunks_per_min == 0` while `refinery_build_queue_length > 0`.

Impact
- Builds are running but logs are not flowing; UI will appear stalled.

Runbook
1) Verify worker log streaming endpoint is reachable.
2) Check control-plane log hub for websocket errors.
3) Confirm log chunk ingestion table growth.
4) Restart worker or control-plane if log stream pipeline is wedged.

### CAS hit rate drop

Signal
- `refinery_cas_hit_rate < 0.20` over a sustained period.

Impact
- Builds are re-downloading or rebuilding artifacts; throughput may drop.

Runbook
1) Check Zot availability and read latency.
2) Confirm worker CAS base URL is correct.
3) Verify CAS artifacts exist for recent packs/runtimes.
4) If CAS is healthy but misses persist, inspect digest mismatches or caching
   logic.

## Runbook quick index

- Build queue backlog -> Builds page, worker logs, queue backend
- Failure spike -> Reason codes, decision trace, hint catalog
- Worker silence -> Worker container + tokens + network
- Database errors -> Postgres health + disk + restore plan
- Log stream stall -> Worker stream + log hub + DB table
- CAS hit rate drop -> Zot health + digests + cache paths

## Prometheus rule file

Import `docs/alerts-prometheus.yml` into your Prometheus instance or use it as a
starting point for an Alertmanager configuration.
