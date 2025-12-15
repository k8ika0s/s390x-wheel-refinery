#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="libffi"
export PACK_VERSION="$LIBFFI_VERSION"
export PACK_SOURCE_URL="$LIBFFI_SOURCE_URL"
export PACK_SOURCE_SHA256="$LIBFFI_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/libffi-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
./configure \
  --prefix="$PREFIX" \
  --libdir="$LIBDIR" \
  --enable-shared \
  --disable-static

make -j"$JOBS"
make install

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
