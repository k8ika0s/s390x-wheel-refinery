#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[worker-entrypoint] %s\n' "$*" >&2
}

PODMAN_BIN="${PODMAN_BIN:-podman}"
CONTAINER_IMAGE="${CONTAINER_IMAGE:-}"
CONTAINER_IMAGE_NATIVE_HEAVY="${CONTAINER_IMAGE_NATIVE_HEAVY:-}"
TLS_VERIFY="${CONTAINER_IMAGE_TLS_VERIFY:-false}"
PULL_RETRIES="${CONTAINER_IMAGE_PULL_RETRIES:-5}"
PULL_POLICY="${CONTAINER_IMAGE_PULL_POLICY:-always}"

pull_builder_image() {
  local image="$1"
  [[ -z "$image" ]] && return 0
  should_pull=1
  case "${PULL_POLICY,,}" in
    if-missing)
      if "$PODMAN_BIN" image exists "$image"; then
        should_pull=0
      fi
      ;;
    always) ;;
    *)
      log "unknown CONTAINER_IMAGE_PULL_POLICY=${PULL_POLICY}; defaulting to always"
      ;;
  esac

  if (( should_pull == 0 )); then
    log "builder image already present: $image"
  else
    pull_args=()
    case "${TLS_VERIFY,,}" in
      1|true|yes|y) pull_args+=(--tls-verify=true) ;;
      *) pull_args+=(--tls-verify=false) ;;
    esac
    attempt=1
    while (( attempt <= PULL_RETRIES )); do
      log "pulling builder image (attempt ${attempt}/${PULL_RETRIES}): $image"
      if "$PODMAN_BIN" pull "${pull_args[@]}" "$image"; then
        log "builder image ready: $image"
        break
      fi
      if (( attempt == PULL_RETRIES )); then
        log "builder image pull failed after ${PULL_RETRIES} attempts; continuing so the worker can expose diagnostics"
        break
      fi
      sleep "$attempt"
      ((attempt++))
    done
  fi
}

if command -v "$PODMAN_BIN" >/dev/null 2>&1; then
  pull_builder_image "$CONTAINER_IMAGE"
  if [[ -n "$CONTAINER_IMAGE_NATIVE_HEAVY" && "$CONTAINER_IMAGE_NATIVE_HEAVY" != "$CONTAINER_IMAGE" ]]; then
    pull_builder_image "$CONTAINER_IMAGE_NATIVE_HEAVY"
  fi
fi

exec /usr/local/bin/worker
