# Log Aggregation Queries

This document provides common queries for analyzing structured logs from the s390x Wheel Refinery system.

## Prerequisites

Logs are output in JSON format with the following structure:
```json
{
  "timestamp": "2026-01-19T20:15:00Z",
  "level": "info",
  "message": "Build completed successfully",
  "correlation_id": "550e8400-e29b-41d4-a716-446655440000",
  "request_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "component": "worker",
  "fields": {
    "package": "numpy",
    "duration": 245.3
  }
}
```

## Using jq for Log Analysis

### Basic Filtering

```bash
# Get all error logs
podman logs go-control-plane 2>&1 | jq 'select(.level == "error")'

# Get logs for specific correlation ID
podman logs go-control-plane 2>&1 | jq 'select(.correlation_id == "550e8400-e29b-41d4-a716-446655440000")'

# Get logs from specific component
podman logs go-control-plane 2>&1 | jq 'select(.component == "api")'

# Get logs within time range
podman logs go-control-plane 2>&1 | jq 'select(.timestamp >= "2026-01-19T20:00:00Z" and .timestamp <= "2026-01-19T21:00:00Z")'
```

### Request Tracing

```bash
# Trace complete request flow by correlation ID
CORR_ID="550e8400-e29b-41d4-a716-446655440000"
podman logs go-control-plane 2>&1 | jq -s "map(select(.correlation_id == \"$CORR_ID\")) | sort_by(.timestamp)"

# Trace request across services
for service in go-control-plane go-worker; do
  echo "=== $service ==="
  podman logs $service 2>&1 | jq "select(.correlation_id == \"$CORR_ID\")"
done

# Get request timeline
podman logs go-control-plane 2>&1 | jq -s "map(select(.correlation_id == \"$CORR_ID\")) | sort_by(.timestamp) | .[] | {timestamp, component, message}"
```

### Error Analysis

```bash
# Count errors by component
podman logs go-control-plane 2>&1 | jq -s 'map(select(.level == "error")) | group_by(.component) | map({component: .[0].component, count: length})'

# Get unique error messages
podman logs go-control-plane 2>&1 | jq -s 'map(select(.level == "error")) | map(.message) | unique'

# Find errors with stack traces
podman logs go-control-plane 2>&1 | jq 'select(.level == "error" and .stack != null)'

# Get error frequency over time (hourly)
podman logs go-control-plane 2>&1 | jq -s 'map(select(.level == "error")) | group_by(.timestamp[0:13]) | map({hour: .[0].timestamp[0:13], count: length})'
```

### Performance Analysis

```bash
# Get slow requests (>5s)
podman logs go-control-plane 2>&1 | jq 'select(.fields.duration > 5)'

# Calculate average duration by endpoint
podman logs go-control-plane 2>&1 | jq -s 'map(select(.fields.duration != null)) | group_by(.fields.path) | map({path: .[0].fields.path, avg_duration: (map(.fields.duration) | add / length)})'

# Find slowest requests
podman logs go-control-plane 2>&1 | jq -s 'map(select(.fields.duration != null)) | sort_by(.fields.duration) | reverse | .[0:10] | .[] | {timestamp, path: .fields.path, duration: .fields.duration}'

# Get p95 duration by endpoint
podman logs go-control-plane 2>&1 | jq -s 'map(select(.fields.duration != null)) | group_by(.fields.path) | map({path: .[0].fields.path, p95: (map(.fields.duration) | sort | .[((length * 0.95) | floor)])})'
```

### Build Analysis

```bash
# Get failed builds
podman logs go-worker 2>&1 | jq 'select(.message | contains("Build failed"))'

# Count builds by status
podman logs go-worker 2>&1 | jq -s 'map(select(.fields.status != null)) | group_by(.fields.status) | map({status: .[0].fields.status, count: length})'

# Get builds for specific package
podman logs go-worker 2>&1 | jq 'select(.fields.package == "numpy")'

# Find long-running builds
podman logs go-worker 2>&1 | jq 'select(.fields.duration > 600)'

# Get build failure reasons
podman logs go-worker 2>&1 | jq -s 'map(select(.level == "error" and .message | contains("Build failed"))) | group_by(.fields.reason) | map({reason: .[0].fields.reason, count: length})'
```

### Worker Health

```bash
# Get worker heartbeats
podman logs go-worker 2>&1 | jq 'select(.message | contains("heartbeat"))'

# Check worker resource usage
podman logs go-worker 2>&1 | jq 'select(.fields.cpu_usage != null or .fields.memory_usage != null)'

# Find worker errors
podman logs go-worker 2>&1 | jq 'select(.level == "error" and .component == "worker")'

# Get worker poll statistics
podman logs go-worker 2>&1 | jq 'select(.message | contains("poll"))'
```

### CAS Operations

```bash
# Get CAS operation logs
podman logs go-worker 2>&1 | jq 'select(.message | contains("CAS"))'

# Count CAS operations by type
podman logs go-worker 2>&1 | jq -s 'map(select(.fields.operation != null)) | group_by(.fields.operation) | map({operation: .[0].fields.operation, count: length})'

# Find slow CAS operations
podman logs go-worker 2>&1 | jq 'select(.fields.operation != null and .fields.duration > 5)'

# Get CAS errors
podman logs go-worker 2>&1 | jq 'select(.level == "error" and .message | contains("CAS"))'
```

## Using grep for Quick Searches

```bash
# Find all errors
podman logs go-control-plane 2>&1 | grep '"level":"error"'

# Search for specific correlation ID
podman logs go-control-plane 2>&1 | grep "550e8400-e29b-41d4-a716-446655440000"

# Find build failures
podman logs go-worker 2>&1 | grep "Build failed"

# Search for specific package
podman logs go-worker 2>&1 | grep '"package":"numpy"'
```

## Exporting Logs for Analysis

```bash
# Export last hour of logs
podman logs --since 1h go-control-plane > control-plane-last-hour.log

# Export logs with timestamps
podman logs --timestamps go-control-plane > control-plane-with-timestamps.log

# Export and filter to JSON only
podman logs go-control-plane 2>&1 | grep '^{' > control-plane-json.log

# Export all service logs
for service in go-control-plane go-worker postgres redis zot minio; do
  podman logs $service > logs/${service}.log 2>&1
done
```

## Log Aggregation with Loki (Optional)

If using Grafana Loki for log aggregation:

### LogQL Queries

```logql
# Get all error logs
{container="go-control-plane"} | json | level="error"

# Trace request by correlation ID
{container=~"go-control-plane|go-worker"} | json | correlation_id="550e8400-e29b-41d4-a716-446655440000"

# Count errors by component
sum by (component) (count_over_time({container="go-control-plane"} | json | level="error" [1h]))

# Get slow requests
{container="go-control-plane"} | json | fields_duration > 5

# Build failure rate
sum(rate({container="go-worker"} | json | message=~"Build failed" [5m])) / sum(rate({container="go-worker"} | json | message=~"Build" [5m]))
```

## Common Troubleshooting Scenarios

### Scenario 1: Investigate Failed Build

```bash
# 1. Find the build failure
podman logs go-worker 2>&1 | jq 'select(.message | contains("Build failed")) | {timestamp, correlation_id, package: .fields.package, reason: .fields.reason}'

# 2. Get full context with correlation ID
CORR_ID="<correlation_id_from_above>"
podman logs go-worker 2>&1 | jq "select(.correlation_id == \"$CORR_ID\")" | jq -s 'sort_by(.timestamp)'

# 3. Check for related errors
podman logs go-worker 2>&1 | jq "select(.correlation_id == \"$CORR_ID\" and .level == \"error\")"
```

### Scenario 2: Debug Slow API Request

```bash
# 1. Find slow requests
podman logs go-control-plane 2>&1 | jq 'select(.fields.duration > 5) | {timestamp, request_id, path: .fields.path, duration: .fields.duration}'

# 2. Trace the slow request
REQ_ID="<request_id_from_above>"
podman logs go-control-plane 2>&1 | jq "select(.request_id == \"$REQ_ID\")" | jq -s 'sort_by(.timestamp)'

# 3. Check database queries
podman logs go-control-plane 2>&1 | jq "select(.request_id == \"$REQ_ID\" and .message | contains(\"query\"))"
```

### Scenario 3: Analyze Worker Health Issues

```bash
# 1. Check recent worker errors
podman logs go-worker --since 1h 2>&1 | jq 'select(.level == "error")'

# 2. Check heartbeat gaps
podman logs go-worker 2>&1 | jq 'select(.message | contains("heartbeat"))' | jq -s 'sort_by(.timestamp) | .[] | .timestamp'

# 3. Check resource usage trends
podman logs go-worker 2>&1 | jq 'select(.fields.cpu_usage != null) | {timestamp, cpu: .fields.cpu_usage, memory: .fields.memory_usage}'
```

### Scenario 4: Track Request Across Services

```bash
# 1. Start with control-plane request
podman logs go-control-plane 2>&1 | jq 'select(.fields.path == "/api/builds") | {correlation_id, request_id, timestamp}'

# 2. Follow to worker
CORR_ID="<correlation_id>"
podman logs go-worker 2>&1 | jq "select(.correlation_id == \"$CORR_ID\")"

# 3. Check CAS operations
podman logs go-worker 2>&1 | jq "select(.correlation_id == \"$CORR_ID\" and .message | contains(\"CAS\"))"
```

## Best Practices

1. **Always use correlation IDs** for request tracing
2. **Include relevant fields** in log entries for filtering
3. **Use structured logging** for machine-readable logs
4. **Set appropriate log levels** (debug in dev, info in prod)
5. **Rotate logs regularly** to prevent disk space issues
6. **Export logs** before troubleshooting complex issues
7. **Use jq for complex analysis**, grep for quick searches
8. **Correlate logs with metrics** from Prometheus/Grafana

## Log Retention

```bash
# Configure log rotation in compose
services:
  go-control-plane:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

## Additional Resources

- [jq Manual](https://stedolan.github.io/jq/manual/)
- [Grafana Loki Documentation](https://grafana.com/docs/loki/latest/)
- [Structured Logging Best Practices](https://www.loggly.com/ultimate-guide/json-logging-best-practices/)

---

**Last Updated**: 2026-01-19
**Maintainer**: Platform Team