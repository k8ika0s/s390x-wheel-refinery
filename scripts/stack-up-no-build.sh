#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
COMPOSE_FILE="${COMPOSE_FILE:-podman-compose.yml}"
DOWN_FIRST="${DOWN_FIRST:-1}"
RUNTIME_ENV_FILE="${RUNTIME_ENV_FILE:-$HOME/.config/refinery/runtime.env}"
PUBLISH_BUILDER_IMAGE="${PUBLISH_BUILDER_IMAGE:-1}"
PUBLISH_WAIT_SECONDS="${PUBLISH_WAIT_SECONDS:-60}"

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

if [[ "$PUBLISH_BUILDER_IMAGE" == "1" && -x "$repo_root/scripts/publish-builder-image.sh" ]]; then
  for _ in $(seq 1 "$PUBLISH_WAIT_SECONDS"); do
    if curl -fsS http://127.0.0.1:5000/v2/_catalog >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  if curl -fsS http://127.0.0.1:5000/v2/_catalog >/dev/null 2>&1; then
    "$repo_root/scripts/publish-builder-image.sh"
    if [[ -n "${CONTAINER_IMAGE_NATIVE_HEAVY:-}" && "${CONTAINER_IMAGE_NATIVE_HEAVY}" != "${CONTAINER_IMAGE:-}" ]]; then
      SOURCE_IMAGE="${SOURCE_IMAGE_NATIVE_HEAVY:-localhost/refinery-builder-native:latest}" \
      TARGET_IMAGE="${CONTAINER_IMAGE_NATIVE_HEAVY}" \
        "$repo_root/scripts/publish-builder-image.sh"
    fi
  else
    echo "warning: zot did not become ready; skipping builder image publish" >&2
  fi
fi
