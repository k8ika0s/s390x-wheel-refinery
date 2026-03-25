#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
COMPOSE_FILE="${COMPOSE_FILE:-podman-compose.yml}"
TAIL_LINES="${TAIL_LINES:-400}"
API_BASE="${API_BASE:-http://localhost:8080}"
OUT_DIR="${OUT_DIR:-output/diagnostics/$(date -u +%Y%m%dT%H%M%SZ)}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mkdir -p "$OUT_DIR"

services=("$@")
if [[ ${#services[@]} -eq 0 ]]; then
  services=(control-plane worker zot minio postgres redis ui prometheus alertmanager grafana postgres-exporter)
fi

"$ENGINE" compose -f "$COMPOSE_FILE" ps > "${OUT_DIR}/compose-ps.txt" 2>&1 || true
"$ENGINE" ps -a > "${OUT_DIR}/podman-ps-a.txt" 2>&1 || true
"$ENGINE" images > "${OUT_DIR}/podman-images.txt" 2>&1 || true

for service in "${services[@]}"; do
  "$ENGINE" compose -f "$COMPOSE_FILE" logs --tail "$TAIL_LINES" "$service" > "${OUT_DIR}/${service}.log" 2>&1 || true
done

curl -sS "${API_BASE}/api/health" > "${OUT_DIR}/api-health.json" 2>&1 || true
curl -sS "${API_BASE}/api/ready" > "${OUT_DIR}/api-ready.json" 2>&1 || true
curl -sS "${API_BASE}/api/metrics" > "${OUT_DIR}/api-metrics.txt" 2>&1 || true
curl -sS "${API_BASE}/metrics" > "${OUT_DIR}/prom-metrics.txt" 2>&1 || true

echo "diagnostics written to ${OUT_DIR}"
