#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="libxml2"
export PACK_VERSION="$LIBXML2_VERSION"
export PACK_SOURCE_URL="$LIBXML2_SOURCE_URL"
export PACK_SOURCE_SHA256="$LIBXML2_SOURCE_SHA256"
export PACK_DEPS="${PACK_DEPS:-zlib xz}"
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc
require_tool pkg-config || true

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/libxml2-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
# Build only the C library and CLI tools (no Python bindings).
./configure \
  --prefix="$PREFIX" \
  --libdir="$LIBDIR" \
  --disable-static \
  --enable-shared \
  --without-python \
  --with-zlib \
  --with-lzma

make -j"$JOBS"
make install

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
