#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
COMPOSE_FILE="${COMPOSE_FILE:-podman-compose.yml}"
DOWN_FIRST="${DOWN_FIRST:-1}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ "$DOWN_FIRST" == "1" ]]; then
  "$ENGINE" compose -f "$COMPOSE_FILE" down --remove-orphans || true
fi

"$ENGINE" compose -f "$COMPOSE_FILE" up -d --force-recreate --no-build
"$ENGINE" compose -f "$COMPOSE_FILE" ps
