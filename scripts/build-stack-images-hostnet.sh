#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
BUILD_NETWORK="${BUILD_NETWORK:-host}"
NO_CACHE="${NO_CACHE:-0}"
IMAGE_PREFIX="${IMAGE_PREFIX:-localhost/s390x-wheel-refinery}"
REBUILD_BASES="${REBUILD_BASES:-0}"
BUILDER_IMAGE="${BUILDER_IMAGE:-refinery-builder:latest}"
BUILDER_BASE_IMAGE="${BUILDER_BASE_IMAGE:-${IMAGE_PREFIX}_builder-base:ubi8}"
WORKER_BASE_IMAGE="${WORKER_BASE_IMAGE:-${IMAGE_PREFIX}_worker-base:ubi8}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

build_args=(build --network "$BUILD_NETWORK")
if [[ "$NO_CACHE" == "1" ]]; then
  build_args+=(--no-cache)
fi

image_exists() {
  "$ENGINE" image exists "$1" >/dev/null 2>&1
}

build_image() {
  local tag="$1"
  local dockerfile="$2"
  shift 2
  echo "==> building ${tag} from ${dockerfile}"
  "$ENGINE" "${build_args[@]}" "$@" -f "$dockerfile" -t "$tag" .
}

ensure_base_image() {
  local tag="$1"
  local dockerfile="$2"
  if [[ "$REBUILD_BASES" == "1" || "$NO_CACHE" == "1" ]] || ! image_exists "$tag"; then
    build_image "$tag" "$dockerfile"
    return
  fi
  echo "==> using cached base ${tag}"
}

services=("$@")
if [[ ${#services[@]} -eq 0 ]]; then
  services=(builder control-plane worker ui zot minio)
fi

for service in "${services[@]}"; do
  case "$service" in
    builder)
      ensure_base_image "$BUILDER_BASE_IMAGE" "containers/refinery-builder-base/Containerfile"
      build_image "$BUILDER_IMAGE" "containers/refinery-builder/Containerfile" \
        --build-arg "BUILDER_BASE_IMAGE=$BUILDER_BASE_IMAGE"
      ;;
    builder-base)
      build_image "$BUILDER_BASE_IMAGE" "containers/refinery-builder-base/Containerfile"
      ;;
    control-plane)
      build_image "${IMAGE_PREFIX}_control-plane:latest" "containers/go-control-plane/Containerfile"
      ;;
    worker)
      ensure_base_image "$WORKER_BASE_IMAGE" "containers/go-worker-base/Containerfile"
      build_image "${IMAGE_PREFIX}_worker:latest" "containers/go-worker/Containerfile" \
        --build-arg "WORKER_BASE_IMAGE=$WORKER_BASE_IMAGE"
      ;;
    worker-base)
      build_image "$WORKER_BASE_IMAGE" "containers/go-worker-base/Containerfile"
      ;;
    ui)
      build_image "${IMAGE_PREFIX}_ui:latest" "containers/ui/Containerfile"
      ;;
    zot)
      build_image "${IMAGE_PREFIX}_zot:latest" "containers/zot/Containerfile"
      ;;
    minio)
      build_image "${IMAGE_PREFIX}_minio:latest" "containers/minio/Containerfile"
      ;;
    *)
      echo "unknown service: ${service}" >&2
      echo "valid services: builder builder-base control-plane worker worker-base ui zot minio" >&2
      exit 1
      ;;
  esac
done
