#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

PY_VERSION="${PY311_VERSION}"
PY_SOURCE_URL="${PY311_SOURCE_URL}"
PY_SOURCE_SHA256="${PY311_SOURCE_SHA256}"

export PACK_NAME="cpython311"
export PACK_VERSION="$PY_VERSION"
export PACK_SOURCE_URL="$PY_SOURCE_URL"
export PACK_SOURCE_SHA256="$PY_SOURCE_SHA256"
export PACK_DEPS="${PACK_DEPS:-openssl zlib libffi bzip2 xz sqlite}"
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool make
require_tool gcc
require_tool g++
require_tool pkg-config || true

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

# Locate OpenSSL prefix from mounted deps.
OPENSSL_PREFIX="${OPENSSL_PREFIX:-}"
if [[ -z "$OPENSSL_PREFIX" ]]; then
  OPENSSL_PREFIX="$(find_dep_prefix_with_file "include/openssl/ssl.h" || true)"
fi
[[ -n "$OPENSSL_PREFIX" ]] || die "OpenSSL headers not found. Provide openssl via DEPS_PREFIXES or set OPENSSL_PREFIX."

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/Python-$PACK_VERSION"

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
# Make the installed python find its libpython without LD_LIBRARY_PATH.
export LDFLAGS="${LDFLAGS:-} -Wl,-rpath,'\\$ORIGIN/../lib'"

./configure \
  --prefix="$PREFIX" \
  --libdir="$LIBDIR" \
  --enable-shared \
  --with-ensurepip=install \
  --with-system-ffi \
  --with-openssl="$OPENSSL_PREFIX"

make -j"$JOBS"
make install

# Optional size trims
if [[ "${KEEP_PYTHON_TESTS:-0}" != "1" ]]; then
  rm -rf "$PREFIX/lib/python"*/test || true
fi
find "$PREFIX" -type d -name '__pycache__' -prune -exec rm -rf {} + 2>/dev/null || true
find "$PREFIX" -type f -name '*.pyc' -delete 2>/dev/null || true

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
