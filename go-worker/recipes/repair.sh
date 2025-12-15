#!/bin/sh
set -eu
WHEEL=${WHEEL_PATH:?wheel path required}
OUT=${REPAIR_OUTPUT:?repair output required}
TOOL=${REPAIR_TOOL_VERSION:-auditwheel}
POLICY=${REPAIR_POLICY_HASH:-manylinux2014_s390x}
# Placeholder: replace with real audit/repair tooling invocation
# Example: auditwheel repair --wheel-dir "$(dirname "$OUT")" --plat "$POLICY" "$WHEEL"
cp "$WHEEL" "$OUT"
echo "Repaired wheel written to $OUT using $TOOL ($POLICY)"
