#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="ninja"
export PACK_VERSION="$NINJA_VERSION"
export PACK_SOURCE_URL="$NINJA_SOURCE_URL"
export PACK_SOURCE_SHA256="$NINJA_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool gcc
require_tool g++
require_tool python3

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"
ninja_dir="$(echo "$src_dir"/ninja-*)"
cd "$ninja_dir"

python3 configure.py --bootstrap

install -d "$PREFIX/bin"
install -m 0755 ninja "$PREFIX/bin/ninja"

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
