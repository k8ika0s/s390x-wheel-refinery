# Alert Runbooks

This document provides detailed troubleshooting procedures and remediation steps for all Prometheus alerts in the s390x Wheel Refinery system.

## Table of Contents

- [Build Alerts](#build-alerts)
- [Worker Alerts](#worker-alerts)
- [Storage Alerts](#storage-alerts)
- [Database Alerts](#database-alerts)
- [System Alerts](#system-alerts)

---

## Build Alerts

### BuildQueueBacklog

**Severity**: Warning (>100 for 5m), Critical (>500 for 10m)

**Description**: Build queue has accumulated too many pending items, indicating processing bottleneck.

**Impact**: Increased wait times for builds, potential user frustration, delayed feedback.

**Diagnosis**:
```bash
# Check current queue depth
curl http://localhost:9090/api/v1/query?query=refinery_queue_depth{queue="build"}

# Check worker availability
curl http://localhost:9090/api/v1/query?query=refinery_workers_online

# Check build throughput
curl http://localhost:9090/api/v1/query?query=rate(refinery_builds_total[5m])
```

**Remediation**:
1. **Immediate** (Critical):
   - Scale up workers: Increase `WORKER_CPU_LIMIT` and `WORKER_MEM_LIMIT`
   - Add more worker instances if infrastructure allows
   - Check for stuck builds: `curl http://localhost:8080/api/builds?status=building`

2. **Short-term** (Warning):
   - Review build duration trends in Grafana
   - Identify slow packages and optimize hints
   - Consider increasing worker pool size via settings

3. **Long-term**:
   - Implement build prioritization
   - Add auto-scaling for workers
   - Optimize pack/runtime caching

**Escalation**: If queue continues growing after scaling, escalate to platform team.

---

### HighBuildFailureRate

**Severity**: Warning (>30% for 10m), Critical (>50% for 5m)

**Description**: Unusually high percentage of builds are failing.

**Impact**: Reduced system effectiveness, potential infrastructure issues, user dissatisfaction.

**Diagnosis**:
```bash
# Check failure rate
curl http://localhost:9090/api/v1/query?query='rate(refinery_builds_failed_total[15m])/rate(refinery_builds_total[15m])'

# Get recent failed builds
curl http://localhost:8080/api/builds?status=failed&limit=20

# Check common failure reasons
curl http://localhost:9090/api/v1/query?query='topk(5,sum by (reason)(rate(refinery_builds_failed_total[1h])))'
```

**Remediation**:
1. **Immediate**:
   - Check system logs for errors: `podman logs go-worker`
   - Verify CAS/MinIO connectivity
   - Check disk space on worker nodes
   - Review recent hint changes

2. **Investigation**:
   - Analyze failure patterns by package
   - Check if specific Python version causing issues
   - Review builder image health
   - Verify network connectivity to PyPI

3. **Resolution**:
   - Update hints for failing packages
   - Roll back recent changes if applicable
   - Rebuild builder image if corrupted
   - Clear cache if stale: `rm -rf ./cache/*`

**Escalation**: If failures persist across all packages, escalate to infrastructure team.

---

### SlowBuilds

**Severity**: Warning (p95 >30m for 15m)

**Description**: Build duration has significantly increased.

**Impact**: Reduced throughput, longer wait times, potential resource exhaustion.

**Diagnosis**:
```bash
# Check build duration percentiles
curl http://localhost:9090/api/v1/query?query='histogram_quantile(0.95,rate(refinery_build_duration_seconds_bucket[15m]))'

# Identify slow packages
curl http://localhost:9090/api/v1/query?query='topk(10,avg by (package)(refinery_build_duration_seconds))'

# Check phase breakdown
curl http://localhost:9090/api/v1/query?query='histogram_quantile(0.95,rate(refinery_build_phase_duration_seconds_bucket[15m]))'
```

**Remediation**:
1. **Immediate**:
   - Check worker resource usage (CPU/memory)
   - Verify CAS performance (check latency metrics)
   - Review concurrent build count

2. **Optimization**:
   - Increase worker resources if constrained
   - Optimize slow build phases (runtime/pack/wheel/repair)
   - Review and update pack dependencies
   - Check for network latency to PyPI

3. **Long-term**:
   - Implement build caching improvements
   - Optimize pack catalog dependencies
   - Consider parallel build phases

**Escalation**: If slowness affects all builds, check infrastructure capacity.

---

## Worker Alerts

### WorkerDown

**Severity**: Critical (offline for 2m)

**Description**: A worker has stopped responding.

**Impact**: Reduced build capacity, potential build failures, queue backlog.

**Diagnosis**:
```bash
# Check worker status
podman ps | grep go-worker

# Check worker logs
podman logs go-worker --tail 100

# Verify worker heartbeat
curl http://localhost:9090/api/v1/query?query='time()-refinery_worker_last_heartbeat_timestamp'
```

**Remediation**:
1. **Immediate**:
   - Restart worker: `podman restart go-worker`
   - Check for OOM kills: `dmesg | grep -i kill`
   - Verify network connectivity

2. **Investigation**:
   - Review worker logs for errors
   - Check resource limits (CPU/memory)
   - Verify Podman socket accessibility
   - Check control-plane connectivity

3. **Prevention**:
   - Increase worker resource limits if needed
   - Implement worker auto-restart
   - Add health check monitoring

**Escalation**: If worker repeatedly crashes, collect logs and escalate to development team.

---

### NoWorkersAvailable

**Severity**: Critical (all workers offline for 1m)

**Description**: All workers are offline - build processing has stopped.

**Impact**: Complete build system outage, no new builds can be processed.

**Diagnosis**:
```bash
# Check all workers
podman ps -a | grep worker

# Check control-plane
podman ps | grep control-plane

# Check network
podman network inspect podman
```

**Remediation**:
1. **Immediate** (P0):
   - Restart all workers: `podman compose restart worker`
   - Check control-plane health: `curl http://localhost:8080/health`
   - Verify Redis/queue backend: `podman logs redis`

2. **System Check**:
   - Check host resources: `free -h`, `df -h`
   - Verify Podman service: `systemctl status podman`
   - Check for network issues
   - Review recent deployments

3. **Recovery**:
   - Full stack restart if needed: `podman compose down && podman compose up -d`
   - Verify queue integrity
   - Requeue stale builds

**Escalation**: Immediate escalation to on-call engineer - this is a P0 incident.

---

### WorkerHighCPU / WorkerHighMemory

**Severity**: Warning (>90% for 10m)

**Description**: Worker resource usage is critically high.

**Impact**: Potential worker crashes, slow builds, system instability.

**Diagnosis**:
```bash
# Check resource usage
podman stats go-worker

# Check active builds
curl http://localhost:9090/api/v1/query?query='refinery_worker_active_builds'

# Check build duration
curl http://localhost:9090/api/v1/query?query='rate(refinery_worker_build_duration_seconds_sum[5m])'
```

**Remediation**:
1. **Immediate**:
   - Review active builds for resource-intensive packages
   - Check for memory leaks in logs
   - Consider reducing concurrent builds temporarily

2. **Scaling**:
   - Increase worker resource limits in compose file
   - Add additional worker instances
   - Implement build timeout enforcement

3. **Optimization**:
   - Profile resource-intensive builds
   - Optimize builder image
   - Implement resource-based build scheduling

**Escalation**: If resource usage remains high after scaling, investigate for memory leaks.

---

### WorkerHighErrorRate

**Severity**: Warning (>1 error/sec for 5m)

**Description**: Worker is experiencing elevated error rate.

**Impact**: Build failures, reduced reliability, potential data corruption.

**Diagnosis**:
```bash
# Check error types
curl http://localhost:9090/api/v1/query?query='rate(refinery_worker_errors_total[5m])'

# Review worker logs
podman logs go-worker --tail 200 | grep -i error

# Check specific error patterns
curl http://localhost:9090/api/v1/query?query='topk(5,sum by (error_type)(rate(refinery_worker_errors_total[5m])))'
```

**Remediation**:
1. **Investigation**:
   - Identify error types from metrics
   - Review logs for stack traces
   - Check external service health (CAS, MinIO, Redis)

2. **Resolution**:
   - Fix configuration issues
   - Restart worker if transient errors
   - Update hints if build-related errors
   - Check network connectivity

3. **Prevention**:
   - Implement retry logic for transient errors
   - Add circuit breakers for external services
   - Improve error handling

**Escalation**: If errors persist, collect logs and escalate to development team.

---

### WorkerMissedHeartbeat

**Severity**: Warning (>120s since last heartbeat for 2m)

**Description**: Worker hasn't sent heartbeat recently.

**Impact**: Potential worker failure, reduced monitoring visibility.

**Diagnosis**:
```bash
# Check heartbeat timestamp
curl http://localhost:9090/api/v1/query?query='time()-refinery_worker_last_heartbeat_timestamp'

# Check worker process
podman top go-worker

# Check network connectivity
curl http://localhost:8080/health
```

**Remediation**:
1. **Immediate**:
   - Check if worker is responsive: `curl http://localhost:8081/health`
   - Review worker logs for blocking operations
   - Check control-plane connectivity

2. **Investigation**:
   - Verify heartbeat interval configuration
   - Check for long-running builds blocking heartbeat
   - Review network latency

3. **Resolution**:
   - Restart worker if unresponsive
   - Adjust heartbeat interval if needed
   - Implement async heartbeat if blocking

**Escalation**: If heartbeats stop completely, treat as WorkerDown.

---

## Storage Alerts

### CASStorageHighUsage / CASStorageCritical

**Severity**: Warning (>80%), Critical (>90%)

**Description**: CAS (Zot registry) storage is running low.

**Impact**: Build failures, inability to store new artifacts, system degradation.

**Diagnosis**:
```bash
# Check storage usage
curl http://localhost:9090/api/v1/query?query='refinery_cas_storage_used_bytes/refinery_cas_storage_total_bytes'

# Check artifact count
curl http://localhost:9090/api/v1/query?query='refinery_cas_artifacts_total'

# Check artifact distribution
curl http://localhost:9090/api/v1/query?query='refinery_cas_artifacts_by_type'
```

**Remediation**:
1. **Immediate** (Critical):
   - Stop non-essential builds
   - Implement emergency cleanup of old artifacts
   - Expand storage volume if possible

2. **Cleanup**:
   ```bash
   # List old artifacts
   curl http://localhost:5000/v2/_catalog
   
   # Implement retention policy
   # Delete artifacts older than 90 days
   # Keep only last N versions of each artifact
   ```

3. **Long-term**:
   - Implement automated retention policies
   - Add storage monitoring and alerting
   - Plan storage capacity expansion
   - Implement artifact compression

**Escalation**: If storage fills completely, escalate to infrastructure team immediately.

---

### MinIOStorageHighUsage / MinIOStorageCritical

**Severity**: Warning (>80%), Critical (>90%)

**Description**: MinIO object storage is running low.

**Impact**: Cannot store new inputs/outputs, build failures, data loss risk.

**Diagnosis**:
```bash
# Check MinIO storage
curl http://localhost:9090/api/v1/query?query='refinery_minio_storage_used_bytes/refinery_minio_storage_total_bytes'

# Check bucket usage
mc du minio/wheelhouse
mc du minio/inputs
```

**Remediation**:
1. **Immediate** (Critical):
   - Archive old wheelhouse outputs
   - Clean up temporary files
   - Expand storage if possible

2. **Cleanup**:
   ```bash
   # Archive old outputs
   mc mirror --older-than 90d minio/wheelhouse /backup/wheelhouse
   mc rm --recursive --older-than 90d minio/wheelhouse
   
   # Clean up failed builds
   mc rm --recursive minio/wheelhouse/failed/
   ```

3. **Long-term**:
   - Implement lifecycle policies
   - Add tiered storage (hot/cold)
   - Automate archival process
   - Monitor growth trends

**Escalation**: Critical storage issues require immediate infrastructure team involvement.

---

### CASHighLatency

**Severity**: Warning (p95 >5s for 10m)

**Description**: CAS operations are taking too long.

**Impact**: Slow builds, timeouts, reduced throughput.

**Diagnosis**:
```bash
# Check CAS latency
curl http://localhost:9090/api/v1/query?query='histogram_quantile(0.95,rate(refinery_cas_operation_duration_seconds_bucket[5m]))'

# Check operation types
curl http://localhost:9090/api/v1/query?query='rate(refinery_cas_operation_duration_seconds_sum[5m])/rate(refinery_cas_operation_duration_seconds_count[5m])'

# Check Zot health
curl http://localhost:5000/v2/_catalog
```

**Remediation**:
1. **Investigation**:
   - Check Zot container resources: `podman stats zot`
   - Review Zot logs: `podman logs zot`
   - Check disk I/O: `iostat -x 1`
   - Verify network latency

2. **Optimization**:
   - Increase Zot resources
   - Optimize storage backend
   - Implement caching layer
   - Check for concurrent operation limits

3. **Long-term**:
   - Implement CDN/caching
   - Optimize artifact sizes
   - Add read replicas

**Escalation**: Persistent latency issues may indicate infrastructure problems.

---

### CASHighErrorRate

**Severity**: Warning (>0.1 errors/sec for 5m)

**Description**: CAS operations are failing frequently.

**Impact**: Build failures, data integrity issues, system instability.

**Diagnosis**:
```bash
# Check error rate
curl http://localhost:9090/api/v1/query?query='rate(refinery_cas_errors_total[5m])'

# Check error types
curl http://localhost:9090/api/v1/query?query='sum by (error_type)(rate(refinery_cas_errors_total[5m]))'

# Check Zot logs
podman logs zot --tail 100 | grep -i error
```

**Remediation**:
1. **Immediate**:
   - Check Zot health: `curl http://localhost:5000/v2/`
   - Verify storage accessibility
   - Check network connectivity
   - Review authentication/authorization

2. **Resolution**:
   - Restart Zot if unhealthy: `podman restart zot`
   - Fix storage permissions
   - Update credentials if expired
   - Check for storage corruption

3. **Prevention**:
   - Implement retry logic
   - Add circuit breakers
   - Improve error handling
   - Monitor storage health

**Escalation**: Data corruption issues require immediate attention.

---

## Database Alerts

### DatabaseConnectionPoolExhausted

**Severity**: Critical (>90% for 5m)

**Description**: Database connection pool is nearly full.

**Impact**: API slowdowns, request failures, system degradation.

**Diagnosis**:
```bash
# Check connection usage
curl http://localhost:9090/api/v1/query?query='pg_stat_database_numbackends/pg_settings_max_connections'

# Check active queries
podman exec postgres psql -U refinery -d refinery -c "SELECT count(*) FROM pg_stat_activity WHERE state='active';"

# Check long-running queries
podman exec postgres psql -U refinery -d refinery -c "SELECT pid, now() - query_start as duration, query FROM pg_stat_activity WHERE state='active' ORDER BY duration DESC LIMIT 10;"
```

**Remediation**:
1. **Immediate**:
   - Kill long-running queries if safe
   - Restart control-plane to reset connections
   - Increase max_connections temporarily

2. **Investigation**:
   - Identify connection leaks in code
   - Review query patterns
   - Check for missing connection closes
   - Analyze connection lifecycle

3. **Long-term**:
   - Implement connection pooling (PgBouncer)
   - Fix connection leaks in code
   - Optimize query performance
   - Add connection timeout enforcement

**Escalation**: If connections remain exhausted, escalate to database team.

---

### DatabaseSlowQueries

**Severity**: Warning (avg >1000ms for 10m)

**Description**: Database queries are taking too long.

**Impact**: API slowdowns, timeouts, poor user experience.

**Diagnosis**:
```bash
# Check query performance
curl http://localhost:9090/api/v1/query?query='rate(pg_stat_statements_mean_exec_time[5m])'

# Identify slow queries
podman exec postgres psql -U refinery -d refinery -c "SELECT query, mean_exec_time, calls FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"

# Check for missing indexes
podman exec postgres psql -U refinery -d refinery -c "SELECT schemaname, tablename, attname, n_distinct, correlation FROM pg_stats WHERE schemaname='public' ORDER BY n_distinct;"
```

**Remediation**:
1. **Investigation**:
   - Identify slow queries
   - Check for missing indexes
   - Review query plans: `EXPLAIN ANALYZE`
   - Check table statistics

2. **Optimization**:
   - Add missing indexes
   - Optimize query structure
   - Update table statistics: `ANALYZE`
   - Consider query caching

3. **Long-term**:
   - Implement query monitoring
   - Add read replicas
   - Optimize schema design
   - Implement caching layer

**Escalation**: Persistent slow queries may require schema redesign.

---

## System Alerts

### HighQueueWaitTime

**Severity**: Warning (p95 >300s for 10m)

**Description**: Items are waiting too long in queue before processing.

**Impact**: Delayed builds, poor user experience, potential timeouts.

**Diagnosis**:
```bash
# Check wait time
curl http://localhost:9090/api/v1/query?query='histogram_quantile(0.95,rate(refinery_queue_wait_duration_seconds_bucket[15m]))'

# Check queue depth
curl http://localhost:9090/api/v1/query?query='refinery_queue_depth'

# Check processing rate
curl http://localhost:9090/api/v1/query?query='rate(refinery_builds_total[5m])'
```

**Remediation**:
1. **Immediate**:
   - Scale up workers
   - Check for stuck items in queue
   - Verify queue backend health (Redis)

2. **Optimization**:
   - Increase worker pool size
   - Implement priority queuing
   - Optimize build duration
   - Add queue monitoring

3. **Long-term**:
   - Implement auto-scaling
   - Add queue sharding
   - Optimize queue backend
   - Implement SLA-based prioritization

**Escalation**: If wait times continue growing, escalate to platform team.

---

### LowHintSuccessRate

**Severity**: Warning (<50% for 30m)

**Description**: Hints are not effectively fixing build failures.

**Impact**: Increased build failures, wasted resources, poor automation.

**Diagnosis**:
```bash
# Check hint success rate
curl http://localhost:9090/api/v1/query?query='rate(refinery_hint_success_total[1h])/rate(refinery_hint_applied_total[1h])'

# Check by hint type
curl http://localhost:9090/api/v1/query?query='rate(refinery_hint_success_total[1h])/rate(refinery_hint_applied_total[1h]) by (hint_type)'

# Review recent hint applications
curl http://localhost:8080/api/builds?status=failed&limit=20
```

**Remediation**:
1. **Investigation**:
   - Identify failing hint types
   - Review hint patterns
   - Check for outdated hints
   - Analyze failure reasons

2. **Improvement**:
   - Update hint patterns
   - Add new hints for common failures
   - Remove ineffective hints
   - Test hints against known failures

3. **Long-term**:
   - Implement hint effectiveness tracking
   - Add ML-based hint generation
   - Create hint testing framework
   - Automate hint updates

**Escalation**: Low hint effectiveness may require hint system redesign.

---

## General Troubleshooting

### Useful Commands

```bash
# Check all service health
podman compose ps

# View all logs
podman compose logs -f

# Check metrics endpoints
curl http://localhost:8080/metrics
curl http://localhost:8081/metrics

# Query Prometheus
curl http://localhost:9090/api/v1/query?query=up

# Check Grafana dashboards
open http://localhost:3001

# Restart specific service
podman compose restart <service>

# Full stack restart
podman compose down && podman compose up -d
```

### Log Locations

- Control-plane: `podman logs go-control-plane`
- Worker: `podman logs go-worker`
- Prometheus: `podman logs prometheus`
- Grafana: `podman logs grafana`
- Alertmanager: `podman logs alertmanager`

### Escalation Contacts

- **P0 (Critical)**: On-call engineer (immediate)
- **P1 (High)**: Platform team (within 1 hour)
- **P2 (Medium)**: Development team (within 4 hours)
- **P3 (Low)**: Create ticket for next sprint

---

## Document Maintenance

**Last Updated**: 2026-01-19
**Owner**: Platform Team
**Review Frequency**: Monthly

**Change Log**:
- 2026-01-19: Initial version with all 20 alerts