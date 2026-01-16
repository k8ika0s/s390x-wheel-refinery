#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <backup_dir>" >&2
  exit 1
fi

backup_root="${1%/}"
if [[ ! -d "${backup_root}" ]]; then
  echo "Backup directory not found: ${backup_root}" >&2
  exit 1
fi

log() {
  printf "[%s] %s\n" "$(date +"%H:%M:%S")" "$*"
}

log "Stopping stack"
podman compose down

for vol in pgdata miniodata zotdata; do
  archive="${backup_root}/${vol}.tar.gz"
  if [[ -f "${archive}" ]]; then
    mount="$(podman volume inspect "${vol}" --format '{{ .Mountpoint }}' 2>/dev/null || true)"
    if [[ -n "${mount}" && -d "${mount}" ]]; then
      log "Restoring volume ${vol}"
      rm -rf "${mount:?}"/*
      tar -C "${mount}" -xzf "${archive}"
    else
      log "Volume ${vol} not found; skipping"
    fi
  fi
done

log "Starting stack"
podman compose up -d

if [[ -f "${backup_root}/postgres.sql" ]]; then
  postgres_id="$(podman compose ps -q postgres 2>/dev/null || true)"
  if [[ -n "${postgres_id}" ]]; then
    log "Restoring Postgres logical dump"
    podman cp "${backup_root}/postgres.sql" "${postgres_id}:/tmp/postgres.sql"
    podman exec -e PGPASSWORD="${POSTGRES_PASSWORD:-refinery}" "${postgres_id}" \
      psql -U "${POSTGRES_USER:-refinery}" -d "${POSTGRES_DB:-refinery}" -f /tmp/postgres.sql
  else
    log "Postgres container not found; skipping postgres.sql restore"
  fi
fi

log "Restore complete"
