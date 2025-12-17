#!/usr/bin/env bash
set -euo pipefail

INPUT_DIR="${INPUT_DIR:-/tmp/refinery-input}"
OUTPUT_DIR="${OUTPUT_DIR:-/output}"
CACHE_DIR="${CACHE_DIR:-/cache}"
PY_TAG="${PYTHON_TAG:-${PYTHON_VERSION:-3.11}}"
PLAT_TAG="${PLATFORM_TAG:-manylinux2014_s390x}"
JOBS="${WORKER_JOBS:-1}"

NAME="${JOB_NAME:-}"
VERSION="${JOB_VERSION:-}"

mkdir -p "$INPUT_DIR"
cmd=(refinery --input "$INPUT_DIR" --output "$OUTPUT_DIR" --cache "$CACHE_DIR" --python "$PY_TAG" --platform-tag "$PLAT_TAG" --jobs "$JOBS")
if [[ -n "$NAME" ]]; then
  if [[ -n "$VERSION" ]]; then
    cmd+=(--only "${NAME}==${VERSION}")
  else
    cmd+=(--only "$NAME")
  fi
fi

exec "${cmd[@]}"
