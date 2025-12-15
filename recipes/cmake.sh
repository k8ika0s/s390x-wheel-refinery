#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="cmake"
export PACK_VERSION="$CMAKE_VERSION"
export PACK_SOURCE_URL="$CMAKE_SOURCE_URL"
export PACK_SOURCE_SHA256="$CMAKE_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc
require_tool g++

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/cmake-$PACK_VERSION"

# CMake bootstraps itself (no cmake required).
# Note: You can tweak these flags if you want to link against your own OpenSSL/cURL/etc.
./bootstrap --prefix="$PREFIX" --parallel="$JOBS"

make -j"$JOBS"
make install

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
