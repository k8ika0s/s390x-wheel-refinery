#!/bin/sh
set -eu

WHEEL=${WHEEL_PATH:?wheel path required}
OUT=${REPAIR_OUTPUT:?repair output required}
POLICY=${REPAIR_POLICY_HASH:-manylinux2014_s390x}
AUDITWHEEL_BIN=${AUDITWHEEL_BIN:-auditwheel}

if [ ! -f "$WHEEL" ]; then
  echo "repair: wheel path not found: $WHEEL" >&2
  exit 1
fi

# Ensure auditwheel is available (best effort pip install if allowed)
if ! command -v "$AUDITWHEEL_BIN" >/dev/null 2>&1; then
  if [ "${AUTO_INSTALL_AUDITWHEEL:-1}" = "1" ]; then
    if command -v pip >/dev/null 2>&1; then
      echo "repair: installing auditwheel..." >&2
      pip install --quiet --no-input auditwheel
    fi
  fi
fi

if ! command -v "$AUDITWHEEL_BIN" >/dev/null 2>&1; then
  echo "repair: auditwheel not available; cannot repair" >&2
  exit 1
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

set +e
"$AUDITWHEEL_BIN" repair --plat "$POLICY" --wheel-dir "$TMPDIR" "$WHEEL"
rc=$?
set -e
if [ $rc -ne 0 ]; then
  echo "repair: auditwheel failed with code $rc" >&2
  exit $rc
fi

newwheel=$(find "$TMPDIR" -maxdepth 1 -type f -name "*.whl" | head -n 1)
if [ -z "$newwheel" ]; then
  echo "repair: no repaired wheel produced" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUT")"
mv "$newwheel" "$OUT"
echo "repair: repaired wheel written to $OUT (policy=$POLICY)"
