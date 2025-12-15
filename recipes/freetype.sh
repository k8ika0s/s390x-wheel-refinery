#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="freetype"
export PACK_VERSION="$FREETYPE_VERSION"
export PACK_SOURCE_URL="$FREETYPE_SOURCE_URL"
export PACK_SOURCE_SHA256="$FREETYPE_SOURCE_SHA256"
export PACK_DEPS="${PACK_DEPS:-zlib libpng bzip2}"
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc
require_tool pkg-config || true

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/freetype-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
# freetype uses pkg-config for zlib/libpng; bzip2 is optional.
./configure \
  --prefix="$PREFIX" \
  --libdir="$LIBDIR" \
  --disable-static \
  --enable-shared \
  --with-zlib=yes \
  --with-png=yes \
  --with-bzip2=yes

make -j"$JOBS"
make install

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
