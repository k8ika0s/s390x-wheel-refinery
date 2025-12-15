#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="sqlite"
export PACK_VERSION="$SQLITE_VERSION"
export PACK_SOURCE_URL="$SQLITE_SOURCE_URL"
export PACK_SOURCE_SHA256="$SQLITE_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
# SQLite "autoconf" tarballs expand to sqlite-autoconf-<num>
sqlite_dir="$(echo "$src_dir"/sqlite-autoconf-*)"
cd "$sqlite_dir"

export CFLAGS="${CFLAGS:-} -O2 -fPIC \
  -DSQLITE_ENABLE_FTS5 \
  -DSQLITE_ENABLE_JSON1 \
  -DSQLITE_ENABLE_RTREE \
  -DSQLITE_ENABLE_COLUMN_METADATA \
  -DSQLITE_USE_URI=1"

./configure \
  --prefix="$PREFIX" \
  --libdir="$LIBDIR" \
  --enable-shared \
  --disable-static \
  --enable-threadsafe \
  --disable-readline

make -j"$JOBS"
make install

# Some builds don't ship a .pc; add one if missing.
if [[ ! -f "$LIBDIR/pkgconfig/sqlite3.pc" && ! -f "$PREFIX/lib/pkgconfig/sqlite3.pc" ]]; then
  install -d "$PREFIX/lib/pkgconfig"
  cat >"$PREFIX/lib/pkgconfig/sqlite3.pc" <<EOF
prefix=$PREFIX
exec_prefix=\${prefix}
libdir=$LIBDIR
includedir=\${prefix}/include

Name: sqlite3
Description: SQLite library
Version: $PACK_VERSION
Libs: -L\${libdir} -lsqlite3
Cflags: -I\${includedir}
EOF
fi

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
