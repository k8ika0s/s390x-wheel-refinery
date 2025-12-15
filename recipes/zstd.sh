#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="zstd"
export PACK_VERSION="$ZSTD_VERSION"
export PACK_SOURCE_URL="$ZSTD_SOURCE_URL"
export PACK_SOURCE_SHA256="$ZSTD_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/zstd-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
# Install library + headers (keep it lean: no programs unless requested)
make -j"$JOBS" -C lib
make -C lib install PREFIX="$PREFIX" LIBDIR="$LIBDIR"

if [[ "${ZSTD_INSTALL_PROGRAMS:-0}" == "1" ]]; then
  make -j"$JOBS"
  make install PREFIX="$PREFIX" LIBDIR="$LIBDIR"
fi

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
