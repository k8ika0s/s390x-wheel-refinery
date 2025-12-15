#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="bzip2"
export PACK_VERSION="$BZIP2_VERSION"
export PACK_SOURCE_URL="$BZIP2_SOURCE_URL"
export PACK_SOURCE_SHA256="$BZIP2_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/bzip2-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
# Build shared + static
make -j"$JOBS" CFLAGS="$CFLAGS"
make -j"$JOBS" -f Makefile-libbz2_so CFLAGS="$CFLAGS"

# Install (bzip2's install target doesn't handle shared lib cleanly; do it explicitly)
install -d "$PREFIX/bin" "$PREFIX/include" "$LIBDIR" "$PREFIX/lib/pkgconfig"

install -m 0755 bzip2-shared "$PREFIX/bin/bzip2" || install -m 0755 bzip2 "$PREFIX/bin/bzip2"
install -m 0644 bzlib.h "$PREFIX/include/bzlib.h"

# Shared libs
if compgen -G "libbz2.so.*" >/dev/null; then
  install -m 0755 libbz2.so.* "$LIBDIR/"
  ln -sf "$(ls -1 libbz2.so.* | head -n1)" "$LIBDIR/libbz2.so"
else
  # Some tarballs name it differently; be a bit forgiving.
  install -m 0755 libbz2.so* "$LIBDIR/" || true
fi

# Static lib (optional; postprocess_prefix removes if KEEP_STATIC!=1)
if [[ -f libbz2.a ]]; then
  install -m 0644 libbz2.a "$LIBDIR/libbz2.a"
fi

# pkg-config file (bzip2 doesn't always ship one)
cat >"$PREFIX/lib/pkgconfig/bzip2.pc" <<EOF
prefix=$PREFIX
exec_prefix=\${prefix}
libdir=$LIBDIR
includedir=\${prefix}/include

Name: bzip2
Description: bzip2 compression library
Version: $PACK_VERSION
Libs: -L\${libdir} -lbz2
Cflags: -I\${includedir}
EOF

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
