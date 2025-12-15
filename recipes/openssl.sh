#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="openssl"
export PACK_VERSION="$OPENSSL_VERSION"
export PACK_SOURCE_URL="$OPENSSL_SOURCE_URL"
export PACK_SOURCE_SHA256="$OPENSSL_SOURCE_SHA256"
# Optional: enable zlib support (set OPENSSL_WITH_ZLIB=1) and provide zlib via DEPS_PREFIXES
export PACK_DEPS="${PACK_DEPS:-zlib}"
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool perl

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
cd "$src_dir/openssl-$PACK_VERSION"

target="${OPENSSL_TARGET:-linux64-s390x}"
conf_opts=(
  "$target"
  "shared"
  "--prefix=$PREFIX"
  "--openssldir=$PREFIX/ssl"
  # keep the output slim and deterministic
  "no-tests"
)

if [[ "${OPENSSL_WITH_ZLIB:-1}" == "1" ]]; then
  conf_opts+=("zlib")
fi

export CFLAGS="${CFLAGS:-} -O2 -fPIC"
./Configure "${conf_opts[@]}"
make -j"$JOBS"
make install_sw install_ssldirs

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
