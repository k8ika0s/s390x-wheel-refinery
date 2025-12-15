#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="pkg-config"
export PACK_VERSION="$PKGCONF_VERSION"
export PACK_SOURCE_URL="$PKGCONF_SOURCE_URL"
export PACK_SOURCE_SHA256="$PKGCONF_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/pkgconf-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
./configure \
  --prefix="$PREFIX" \
  --libdir="$LIBDIR" \
  --disable-static \
  --enable-shared

make -j"$JOBS"
make install

# Provide the common pkg-config entrypoint name.
if [[ -x "$PREFIX/bin/pkgconf" && ! -e "$PREFIX/bin/pkg-config" ]]; then
  ln -s pkgconf "$PREFIX/bin/pkg-config"
fi

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
