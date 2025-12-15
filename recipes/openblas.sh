#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="openblas"
export PACK_VERSION="$OPENBLAS_VERSION"
export PACK_SOURCE_URL="$OPENBLAS_SOURCE_URL"
export PACK_SOURCE_SHA256="$OPENBLAS_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc
require_tool gfortran

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/OpenBLAS-$PACK_VERSION"

# Fedora and SUSE builds pin s390x to the generic z/Architecture baseline:
#   TARGET=ZARCH_GENERIC DYNAMIC_ARCH=1 DYNAMIC_OLDER=1
# (See upstream packagers for rationale.)
target_opts=(
  "TARGET=ZARCH_GENERIC"
  "DYNAMIC_ARCH=1"
  "DYNAMIC_OLDER=1"
)

num_threads="${OPENBLAS_NUM_THREADS:-64}"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
# Build threaded shared library (no OpenMP).
make -j"$JOBS" \
  "${target_opts[@]}" \
  USE_THREAD=1 USE_OPENMP=0 USE_LOCKING=1 \
  NUM_THREADS="$num_threads"

make install PREFIX="$PREFIX"

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
