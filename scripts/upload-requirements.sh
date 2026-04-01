#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

API_BASE="${API_BASE:-http://localhost:8080}"
REQ_FILE="${REQ_FILE:-}"
REQ_FILENAME="${REQ_FILENAME:-}"
UI_TOKEN="${UI_TOKEN:-}"
ENQUEUE_PLAN="${ENQUEUE_PLAN:-auto}"

if [[ -z "$REQ_FILE" ]]; then
  echo "REQ_FILE is required." >&2
  exit 1
fi
if [[ ! -f "$REQ_FILE" ]]; then
  echo "REQ_FILE not found: ${REQ_FILE}" >&2
  exit 1
fi

req_name="$REQ_FILENAME"
if [[ -z "$req_name" ]]; then
  req_name="$(basename "$REQ_FILE")"
fi

curl_args=(-sS -f)
if [[ -n "$UI_TOKEN" ]]; then
  curl_args+=(-H "X-UI-Token: ${UI_TOKEN}")
fi

upload_resp="$(
  curl "${curl_args[@]}" -X POST \
    -F "file=@${REQ_FILE};filename=${req_name}" \
    "${API_BASE}/api/requirements/upload"
)"

echo "$upload_resp"

pending_id="$(python3 - <<'PY' "$upload_resp"
import json, sys
data = json.loads(sys.argv[1])
print(data.get("pending_id") or "")
PY
)"

if [[ -z "$pending_id" ]]; then
  echo "requirements upload did not return a pending_id" >&2
  exit 1
fi

should_enqueue="$ENQUEUE_PLAN"
if [[ "$ENQUEUE_PLAN" == "auto" ]]; then
  auto_plan="$(
    curl "${curl_args[@]}" "${API_BASE}/api/settings" | python3 - <<'PY'
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    print("")
    raise SystemExit(0)
print("1" if data.get("auto_plan") else "0")
PY
  )"
  if [[ "$auto_plan" == "1" ]]; then
    should_enqueue="0"
  else
    should_enqueue="1"
  fi
fi

if [[ "$should_enqueue" != "1" ]]; then
  echo "auto_plan is enabled; skipped manual enqueue for pending input ${pending_id}"
  exit 0
fi

curl "${curl_args[@]}" -X POST \
  "${API_BASE}/api/pending-inputs/${pending_id}/enqueue-plan" >/dev/null

echo "enqueued pending input ${pending_id} for planning"
