#!/usr/bin/env bash
set -euo pipefail

backup_root="${1:-backups/$(date +"%Y%m%d-%H%M%S")}"
backup_root="${backup_root%/}"
mkdir -p "${backup_root}"

log() {
  printf "[%s] %s\n" "$(date +"%H:%M:%S")" "$*"
}

postgres_id="$(podman compose ps -q postgres 2>/dev/null || true)"
if [[ -n "${postgres_id}" ]]; then
  log "Backing up Postgres (logical dump)"
  podman exec -e PGPASSWORD="${POSTGRES_PASSWORD:-refinery}" "${postgres_id}" \
    pg_dump -U "${POSTGRES_USER:-refinery}" -d "${POSTGRES_DB:-refinery}" > "${backup_root}/postgres.sql"
else
  log "Postgres container not found; skipping postgres.sql"
fi

for vol in pgdata miniodata zotdata; do
  mount="$(podman volume inspect "${vol}" --format '{{ .Mountpoint }}' 2>/dev/null || true)"
  if [[ -n "${mount}" && -d "${mount}" ]]; then
    log "Archiving volume ${vol}"
    tar -C "${mount}" -czf "${backup_root}/${vol}.tar.gz" .
  else
    log "Volume ${vol} not found; skipping"
  fi
done

cat <<EOF_META > "${backup_root}/manifest.txt"
backup_time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
pg_dump_file=postgres.sql
volumes=pgdata,miniodata,zotdata
EOF_META

log "Backup complete: ${backup_root}"
