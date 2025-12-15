#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="jpeg"
export PACK_VERSION="$LIBJPEG_TURBO_VERSION"
export PACK_SOURCE_URL="$LIBJPEG_TURBO_SOURCE_URL"
export PACK_SOURCE_SHA256="$LIBJPEG_TURBO_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool cmake
require_tool make
require_tool gcc
require_tool g++

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
# GitHub tag tarball expands to libjpeg-turbo-<tag>
jpeg_dir="$(echo "$src_dir"/libjpeg-turbo-*)"
cd "$jpeg_dir"

mkdir -p build
cd build

cmake -G "Unix Makefiles" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR="lib" \
  -DENABLE_STATIC=FALSE \
  -DENABLE_SHARED=TRUE \
  -DWITH_JPEG8=TRUE \
  -DWITH_TURBOJPEG=TRUE \
  ..

make -j"$JOBS"
make install

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
