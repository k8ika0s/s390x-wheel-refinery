#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

ENGINE="${ENGINE:-podman}"
TARGET_IMAGE="${TARGET_IMAGE:-127.0.0.1:5000/refinery-builder:latest}"

if [[ -z "${SOURCE_IMAGE:-}" ]]; then
  for candidate in localhost/refinery-builder:latest refinery-builder:latest; do
    if "$ENGINE" image exists "$candidate"; then
      SOURCE_IMAGE="$candidate"
      break
    fi
  done
fi

if [[ -z "${SOURCE_IMAGE:-}" ]]; then
  echo "builder image not found; expected localhost/refinery-builder:latest or refinery-builder:latest" >&2
  exit 1
fi

echo "==> tagging ${SOURCE_IMAGE} as ${TARGET_IMAGE}"
"$ENGINE" tag "$SOURCE_IMAGE" "$TARGET_IMAGE"

echo "==> pushing ${TARGET_IMAGE}"
"$ENGINE" push --tls-verify=false "$TARGET_IMAGE"
