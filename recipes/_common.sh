#!/usr/bin/env bash
set -euo pipefail

# Common helpers for s390x build-pack recipes.
# Contract:
#   - Each recipe is executed with PACK_OUTPUT pointing at an existing, empty dir.
#   - Recipes install into: $PACK_OUTPUT/usr/local (override via PACK_PREFIX_REL if needed).
#   - Recipes should be deterministic-ish (SOURCE_DATE_EPOCH, stable flags, no host leakage).

log() { printf '\n[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }
die() { echo "error: $*" >&2; exit 1; }

require_var() {
  local name="$1"
  [[ -n "${!name:-}" ]] || die "env var '$name' must be set"
}

require_tool() {
  local t="$1"
  command -v "$t" >/dev/null 2>&1 || die "required tool not found in PATH: $t"
}

nproc_() {
  if command -v nproc >/dev/null 2>&1; then nproc
  elif command -v getconf >/dev/null 2>&1; then getconf _NPROCESSORS_ONLN
  else echo 2
  fi
}

script_dir() {
  # Directory of the currently running recipe script (not this file).
  local src="${BASH_SOURCE[0]}"
  while [[ -h "$src" ]]; do
    src="$(readlink "$src")"
  done
  cd "$(dirname "$src")" && pwd
}

init_repro_env() {
  export LC_ALL="${LC_ALL:-C}"
  export LANG="${LANG:-C}"
  export TZ="${TZ:-UTC}"
  umask 0022
  # If your build system already sets SOURCE_DATE_EPOCH, great; otherwise pick a stable default.
  export SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-1700000000}"
  export ARFLAGS="${ARFLAGS:-crD}"  # deterministic static archives (GNU ar)
  export MAKEFLAGS="${MAKEFLAGS:-}"
  export JOBS="${JOBS:-$(nproc_)}"
}

ensure_empty_dir() {
  local d="$1"
  [[ -d "$d" ]] || die "expected directory: $d"
  # Allow dotfiles, but require "no payload".
  if [[ -n "$(ls -A "$d")" ]]; then
    die "PACK_OUTPUT must be empty; found contents in: $d"
  fi
}

pack_prefix() {
  local out="$1"
  local rel="${PACK_PREFIX_REL:-usr/local}"
  echo "${out%/}/${rel}"
}

libdir_for_prefix() {
  local prefix="$1"
  # Force lib (not lib64) for predictable layouts.
  echo "$prefix/lib"
}

# Dependency prefixes (already-mounted packs) can be provided as:
#   DEPS_PREFIXES="/opt/packs/<digest1>/usr/local:/opt/packs/<digest2>/usr/local"
# We'll wire these into PATH/PKG_CONFIG_PATH/CPPFLAGS/LDFLAGS/LD_LIBRARY_PATH.
setup_deps_env() {
  local prefixes="${DEPS_PREFIXES:-}"
  [[ -n "$prefixes" ]] || return 0

  local IFS=':'
  for p in $prefixes; do
    [[ -d "$p" ]] || die "DEPS_PREFIXES entry does not exist: $p"
    export PATH="$p/bin:$PATH"
    export PKG_CONFIG_PATH="$p/lib/pkgconfig:$p/share/pkgconfig:${PKG_CONFIG_PATH:-}"
    export CPPFLAGS="-I$p/include ${CPPFLAGS:-}"
    export CFLAGS="-I$p/include ${CFLAGS:-}"
    export CXXFLAGS="-I$p/include ${CXXFLAGS:-}"
    export LDFLAGS="-L$p/lib -L$p/lib64 ${LDFLAGS:-}"
    export LD_LIBRARY_PATH="$p/lib:$p/lib64:${LD_LIBRARY_PATH:-}"
  done
}

# Add reproducible path mapping once WORKDIR exists.
setup_repro_flags_for_workdir() {
  local workdir="$1"
  # Avoid leaking build paths into debug info / __FILE__ strings.
  export CFLAGS="${CFLAGS:-} -ffile-prefix-map=$workdir=/usr/src -fdebug-prefix-map=$workdir=/usr/src"
  export CXXFLAGS="${CXXFLAGS:-} -ffile-prefix-map=$workdir=/usr/src -fdebug-prefix-map=$workdir=/usr/src"
  export FFLAGS="${FFLAGS:-} -ffile-prefix-map=$workdir=/usr/src -fdebug-prefix-map=$workdir=/usr/src"
}



find_dep_prefix_with_file() {
  # Find the first dep prefix that contains the given relative path.
  # Example: find_dep_prefix_with_file "include/openssl/ssl.h"
  local rel="$1"
  local prefixes="${DEPS_PREFIXES:-}"
  [[ -n "$prefixes" ]] || return 1
  local IFS=':'
  for p in $prefixes; do
    if [[ -e "$p/$rel" ]]; then
      echo "$p"
      return 0
    fi
  done
  return 1
}

# Download helpers
fetch() {
  local url="$1"
  local out="$2"

  require_tool sha256sum

  if [[ -f "$out" ]]; then
    log "using cached source: $out"
    return 0
  fi

  if command -v curl >/dev/null 2>&1; then
    log "downloading: $url"
    curl -fL --retry 3 --retry-delay 2 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    log "downloading: $url"
    wget -O "$out" "$url"
  else
    die "need curl or wget to fetch sources"
  fi
}

verify_sha256() {
  local file="$1"
  local expected="$2"
  if [[ -z "$expected" ]]; then
    if [[ "${ALLOW_MISSING_SHA256:-0}" == "1" ]]; then
      log "WARNING: skipping sha256 verification for $file (ALLOW_MISSING_SHA256=1)"
      return 0
    fi
    die "no expected sha256 provided for $file (set PACK_SOURCE_SHA256 or ALLOW_MISSING_SHA256=1)"
  fi
  echo "${expected}  ${file}" | sha256sum -c -
}

extract() {
  local archive="$1"
  local dest="$2"
  mkdir -p "$dest"

  case "$archive" in
    *.tar.gz|*.tgz) tar -xzf "$archive" -C "$dest" ;;
    *.tar.xz)       tar -xJf "$archive" -C "$dest" ;;
    *.tar.bz2)      tar -xjf "$archive" -C "$dest" ;;
    *.zip)          require_tool unzip; unzip -q "$archive" -d "$dest" ;;
    *)
      die "don't know how to extract: $archive"
      ;;
  esac
}

# Post-install hygiene: keep outputs lean and predictable.
postprocess_prefix() {
  local prefix="$1"
  # Remove libtool archives
  find "$prefix" -type f -name '*.la' -delete || true

  # Remove docs/manpages unless explicitly kept
  if [[ "${KEEP_DOCS:-0}" != "1" ]]; then
    rm -rf "$prefix/share/man" "$prefix/share/doc" "$prefix/share/info" || true
  fi

  # Optionally prune static libs
  if [[ "${KEEP_STATIC:-0}" != "1" ]]; then
    find "$prefix" -type f -name '*.a' -delete || true
  fi

  # Strip shared objects (best-effort; some toolchains don't like stripping)
  if command -v strip >/dev/null 2>&1; then
    find "$prefix" -type f \( -name '*.so' -o -name '*.so.*' \) -exec strip --strip-unneeded {} + 2>/dev/null || true
  fi
}

compute_recipe_digest() {
  # Hash the *contents* of the given files, in order (no filenames in the digest).
  require_tool sha256sum
  local files=("$@")
  cat "${files[@]}" | sha256sum | awk '{print $1}'
}

emit_manifest_json() {
  local out_dir="$1"
  local prefix_rel="${PACK_PREFIX_REL:-usr/local}"

  local deps_json="[]"
  if [[ -n "${PACK_DEPS:-}" ]]; then
    deps_json="["
    local d
    for d in $PACK_DEPS; do
      deps_json+="\"$d\","
    done
    deps_json="${deps_json%,}]"
  fi

  cat >"${out_dir%/}/manifest.json" <<EOF
{
  "name": "${PACK_NAME}",
  "version": "${PACK_VERSION}",
  "policy": "${PACK_POLICY:-unknown}",
  "recipe_digest": "${RECIPE_DIGEST:-unknown}",
  "source": "${PACK_SOURCE_URL:-}",
  "source_sha256": "${PACK_SOURCE_SHA256:-}",
  "deps": ${deps_json},
  "prefix_rel": "${prefix_rel}",
  "built_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
}

pack_start() {
  init_repro_env
  require_var PACK_OUTPUT
  ensure_empty_dir "$PACK_OUTPUT"

  # Recipe should set PACK_NAME and PACK_VERSION (and optionally PACK_POLICY etc)
  require_var PACK_NAME
  require_var PACK_VERSION

  export PACK_POLICY="${PACK_POLICY:-manylinux2014_s390x}"
  export PREFIX="$(pack_prefix "$PACK_OUTPUT")"
  export LIBDIR="$(libdir_for_prefix "$PREFIX")"

  mkdir -p "$PREFIX" "$LIBDIR"

  setup_deps_env

  log "pack: ${PACK_NAME}@${PACK_VERSION} (policy=${PACK_POLICY})"
  log "install prefix: $PREFIX"
  log "jobs: ${JOBS}"
}

pack_fetch_and_extract() {
  require_var PACK_SOURCE_URL
  require_var PACK_SOURCE_SHA256

  local workdir="$1"
  local archive_name="${2:-source.tar.gz}"

  local sources_dir="${SOURCES_DIR:-}"
  local archive_path="$workdir/$archive_name"
  if [[ -n "$sources_dir" ]]; then
    mkdir -p "$sources_dir"
    archive_path="$sources_dir/${archive_name}"
  fi

  fetch "$PACK_SOURCE_URL" "$archive_path"
  verify_sha256 "$archive_path" "$PACK_SOURCE_SHA256"

  local src_dir="$workdir/src"
  extract "$archive_path" "$src_dir"

  echo "$src_dir"
}
