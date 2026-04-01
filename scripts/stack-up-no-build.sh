#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
COMPOSE_FILE="${COMPOSE_FILE:-podman-compose.yml}"
DOWN_FIRST="${DOWN_FIRST:-1}"
RUNTIME_ENV_FILE="${RUNTIME_ENV_FILE:-$HOME/.config/refinery/runtime.env}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ -f "$RUNTIME_ENV_FILE" ]]; then
  # Export remote runtime overrides such as inference settings and builder wiring.
  set -a
  # shellcheck disable=SC1090
  source "$RUNTIME_ENV_FILE"
  REFINERY_RUNTIME_ENV_LOADED=1
  REFINERY_RUNTIME_ENV_SOURCE="$RUNTIME_ENV_FILE"
  set +a
else
  export REFINERY_RUNTIME_ENV_LOADED=0
  export REFINERY_RUNTIME_ENV_SOURCE="process-env"
fi

if [[ "$DOWN_FIRST" == "1" ]]; then
  "$ENGINE" compose -f "$COMPOSE_FILE" down --remove-orphans || true
fi

"$ENGINE" compose -f "$COMPOSE_FILE" up -d --force-recreate --no-build
"$ENGINE" compose -f "$COMPOSE_FILE" ps
