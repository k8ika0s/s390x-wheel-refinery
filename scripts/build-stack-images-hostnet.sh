#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
BUILD_NETWORK="${BUILD_NETWORK:-host}"
NO_CACHE="${NO_CACHE:-0}"
IMAGE_PREFIX="${IMAGE_PREFIX:-localhost/s390x-wheel-refinery}"
BUILDER_IMAGE="${BUILDER_IMAGE:-refinery-builder:latest}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

build_args=(build --network "$BUILD_NETWORK")
if [[ "$NO_CACHE" == "1" ]]; then
  build_args+=(--no-cache)
fi

build_image() {
  local tag="$1"
  local dockerfile="$2"
  echo "==> building ${tag} from ${dockerfile}"
  "$ENGINE" "${build_args[@]}" -f "$dockerfile" -t "$tag" .
}

services=("$@")
if [[ ${#services[@]} -eq 0 ]]; then
  services=(builder control-plane worker ui zot minio)
fi

for service in "${services[@]}"; do
  case "$service" in
    builder)
      build_image "$BUILDER_IMAGE" "containers/refinery-builder/Containerfile"
      ;;
    control-plane)
      build_image "${IMAGE_PREFIX}_control-plane:latest" "containers/go-control-plane/Containerfile"
      ;;
    worker)
      build_image "${IMAGE_PREFIX}_worker:latest" "containers/go-worker/Containerfile"
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
      echo "valid services: builder control-plane worker ui zot minio" >&2
      exit 1
      ;;
  esac
done
