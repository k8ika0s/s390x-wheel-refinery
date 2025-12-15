#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/_common.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/versions.sh"

export PACK_NAME="rust"
export PACK_VERSION="$RUST_VERSION"
export PACK_SOURCE_URL="$RUST_SOURCE_URL"
export PACK_SOURCE_SHA256="$RUST_SOURCE_SHA256"
export PACK_DEPS=""
export RECIPE_DIGEST="$(compute_recipe_digest "$0" "$SCRIPT_DIR/_common.sh" "$SCRIPT_DIR/versions.sh")"

pack_start
require_tool tar
require_tool xz
require_tool bash

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
setup_repro_flags_for_workdir "$WORKDIR"

src_dir="$(pack_fetch_and_extract "$WORKDIR" "$(basename "$PACK_SOURCE_URL")")"

rust_dir="$src_dir/rust-${PACK_VERSION}-${RUST_TARGET_TRIPLE}"
cd "$rust_dir"

# Standalone Rust installers ship an install.sh that installs into a configurable prefix
# and can skip ldconfig (important in container/pack builds).
bash ./install.sh --prefix="$PREFIX" --disable-ldconfig

postprocess_prefix "$PREFIX"
emit_manifest_json "$PACK_OUTPUT"
log "done: $PACK_NAME"
