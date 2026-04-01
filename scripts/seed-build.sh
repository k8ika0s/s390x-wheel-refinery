#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

API_BASE="${API_BASE:-http://localhost:8080}"
PACKAGE="${PACKAGE:-six}"
VERSION="${VERSION:-1.16.0}"
REQ_LINE="${REQ_LINE:-}"
REQ_FILE="${REQ_FILE:-}"
REQ_FILENAME="${REQ_FILENAME:-}"
UI_TOKEN="${UI_TOKEN:-}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-180}"
POLL_INTERVAL="${POLL_INTERVAL:-2}"
VERIFY_ARTIFACTS="${VERIFY_ARTIFACTS:-1}"
MANIFEST_LIMIT="${MANIFEST_LIMIT:-200}"

if [[ -z "$REQ_LINE" ]]; then
  if [[ -n "$VERSION" ]]; then
    REQ_LINE="${PACKAGE}==${VERSION}"
  else
    REQ_LINE="${PACKAGE}"
  fi
fi

if [[ -z "$PACKAGE" ]]; then
  echo "PACKAGE is required." >&2
  exit 1
fi

tmp_req="$(mktemp)"
cleanup() {
  rm -f "$tmp_req" "${after_file:-}" "${after_seq_file:-}"
}
trap cleanup EXIT

curl_ui_args=()
if [[ -n "$UI_TOKEN" ]]; then
  curl_ui_args+=(-H "X-UI-Token: ${UI_TOKEN}")
fi

check_url() {
  local url="$1"
  if curl -sSfI "$url" >/dev/null 2>&1; then
    return 0
  fi
  curl -sSf -r 0-0 "$url" >/dev/null
}

req_upload_file="$tmp_req"
req_upload_name="requirements-seed.txt"
if [[ -n "$REQ_FILE" ]]; then
  if [[ ! -f "$REQ_FILE" ]]; then
    echo "REQ_FILE not found: ${REQ_FILE}" >&2
    exit 1
  fi
  req_upload_file="$REQ_FILE"
  if [[ -n "$REQ_FILENAME" ]]; then
    req_upload_name="$REQ_FILENAME"
  else
    req_upload_name="$(basename "$REQ_FILE")"
  fi
else
  printf "%s\n" "$REQ_LINE" > "$tmp_req"
fi

if [[ -n "$REQ_FILE" ]]; then
  echo "Uploading requirements file: ${req_upload_file}"
else
  echo "Uploading requirements: ${REQ_LINE}"
fi
upload_resp="$(curl -sS -f "${curl_ui_args[@]}" -X POST -F "file=@${req_upload_file};filename=${req_upload_name}" "${API_BASE}/api/requirements/upload")"
pending_id="$(python3 - <<'PY' "$upload_resp"
import json,sys
data=json.loads(sys.argv[1])
pid=data.get("pending_id") or 0
print(pid)
PY
)"

if [[ -z "$pending_id" || "$pending_id" == "0" ]]; then
  echo "Failed to register pending input." >&2
  echo "$upload_resp" >&2
  exit 1
fi

echo "Pending input id: ${pending_id}"

if ! curl -sS -f "${curl_ui_args[@]}" -X POST "${API_BASE}/api/pending-inputs/${pending_id}/enqueue-plan" >/dev/null; then
  echo "Plan enqueue request failed (plan queue may be auto). Continuing." >&2
fi

plan_id=""
deadline=$(( $(date +%s) + TIMEOUT_SECONDS ))

while [[ $(date +%s) -lt $deadline ]]; do
  for status in planned queued build_queued planning; do
    plan_id="$(curl -sS "${API_BASE}/api/pending-inputs?status=${status}" | python3 - "$pending_id" <<'PY'
import json,sys
pid=int(sys.argv[1])
items=json.load(sys.stdin)
for item in items:
    if int(item.get("id") or 0) == pid:
        plan_id=item.get("plan_id")
        if plan_id:
            print(plan_id)
        else:
            print("")
        break
PY
)"
    if [[ -n "$plan_id" ]]; then
      break 2
    fi
  done
  sleep "$POLL_INTERVAL"
done

case "${status:-}" in
  built|cached|reused)
    ;;
  *)
    echo "Build did not succeed (status: ${status:-unknown})." >&2
    exit 1
    ;;
esac

if [[ "$VERIFY_ARTIFACTS" == "1" ]]; then
  echo "Verifying manifest and artifact URLs..."
  manifest_info="$(curl -sS "${API_BASE}/api/manifest?limit=${MANIFEST_LIMIT}" | python3 - "$PACKAGE" "$VERSION" <<'PY'
import json,sys
pkg=sys.argv[1].lower()
ver=sys.argv[2]
data=json.load(sys.stdin)
match=None
for item in data:
    if str(item.get("name","")).lower() == pkg and str(item.get("version","")) == ver:
        match=item
        break
if not match:
    print("")
    raise SystemExit
status=match.get("status") or ""
wheel=match.get("wheel_url") or match.get("wheel") or ""
repair=match.get("repair_url") or ""
print(f"{status}|{wheel}|{repair}")
PY
)"
  if [[ -z "$manifest_info" ]]; then
    echo "No manifest entry found for ${PACKAGE} ${VERSION}." >&2
    exit 1
  fi
  manifest_status="${manifest_info%%|*}"
  rest="${manifest_info#*|}"
  wheel_url="${rest%%|*}"
  repair_url="${rest#*|}"
  if [[ -n "$manifest_status" && "$manifest_status" != "built" && "$manifest_status" != "cached" && "$manifest_status" != "reused" ]]; then
    echo "Manifest status not successful: ${manifest_status}" >&2
    exit 1
  fi
  if [[ -z "$wheel_url" ]]; then
    echo "Manifest entry missing wheel URL." >&2
    exit 1
  fi
  if ! check_url "$wheel_url"; then
    echo "Wheel URL not reachable: ${wheel_url}" >&2
    exit 1
  fi
  if [[ -n "$repair_url" ]]; then
    if ! check_url "$repair_url"; then
      echo "Repair URL not reachable: ${repair_url}" >&2
      exit 1
    fi
  fi
  echo "Artifact verification complete."
fi

if [[ -z "$plan_id" ]]; then
  echo "Plan id not linked to pending input. Falling back to latest plan." >&2
  start_ts=$(( $(date +%s) - TIMEOUT_SECONDS ))
  plan_id="$(curl -sS "${API_BASE}/api/plans?limit=5" | python3 - "$start_ts" <<'PY'
import json,sys
start=int(sys.argv[1])
plans=json.load(sys.stdin)
best=None
for p in plans:
    created=int(p.get("created_at") or 0)
    if created >= start and (best is None or created > best.get("created_at", 0)):
        best=p
if best:
    print(best.get("id") or "")
PY
)"
fi

if [[ -z "$plan_id" ]]; then
  echo "Unable to find plan id." >&2
  exit 1
fi

echo "Plan id: ${plan_id}"
enqueue_resp="$(curl -sS -f "${curl_ui_args[@]}" -X POST "${API_BASE}/api/plan/${plan_id}/enqueue-builds")"
echo "Builds enqueued: ${enqueue_resp}"

if [[ -n "${WORKER_TOKEN:-}" ]]; then
  curl -sS -f -X POST -H "X-Worker-Token: ${WORKER_TOKEN}" "${API_BASE}/api/worker/trigger" >/dev/null || true
fi

after_file="$(mktemp)"
after_seq_file="$(mktemp)"
echo "Tailing logs for ${PACKAGE} ${VERSION}..."
prev_status=""

while true; do
  build_info="$(curl -sS "${API_BASE}/api/builds?package=${PACKAGE}&version=${VERSION}&limit=5" | python3 - <<'PY'
import json,sys
items=json.load(sys.stdin)
if not items:
    print("")
    raise SystemExit
def stamp(item):
    return int(item.get("updated_at") or item.get("created_at") or 0)
item=max(items, key=stamp)
status=item.get("status") or ""
attempts=item.get("attempts") or 0
updated=item.get("updated_at") or item.get("created_at") or 0
print(f"{status}|{attempts}|{updated}")
PY
)"
  if [[ -n "$build_info" ]]; then
    status="${build_info%%|*}"
    rest="${build_info#*|}"
    attempts="${rest%%|*}"
    if [[ "$status" != "$prev_status" ]]; then
      echo "Status: ${status} (attempts: ${attempts})"
      prev_status="$status"
    fi
  else
    echo "Waiting for build record..."
  fi

  after_id="$(cat "$after_file" 2>/dev/null || echo 0)"
  after_seq="$(cat "$after_seq_file" 2>/dev/null || echo 0)"
  log_attempt="${attempts:-0}"
  query_parts=()
  if [[ "$after_seq" != "0" ]]; then
    query_parts+=("after_seq=${after_seq}")
  elif [[ "$after_id" != "0" ]]; then
    query_parts+=("after=${after_id}")
  fi
  if [[ -n "$log_attempt" && "$log_attempt" != "0" ]]; then
    query_parts+=("attempt=${log_attempt}")
  fi
  query_parts+=("limit=200")
  log_query="$(IFS='&'; echo "${query_parts[*]}")"

  curl -sS "${API_BASE}/api/logs/chunks/${PACKAGE}/${VERSION}?${log_query}" \
    | python3 - "$after_file" "$after_seq_file" <<'PY'
import json,sys
after_file=sys.argv[1]
after_seq_file=sys.argv[2]
data=json.load(sys.stdin)
last_id=0
last_seq=0
out=[]
for chunk in data:
    if not chunk:
        continue
    content=chunk.get("content") or ""
    if content:
        out.append(content)
    cid=chunk.get("id")
    if cid:
        last_id=cid
    seq=chunk.get("seq") or 0
    if seq:
        last_seq=seq
if out:
    for entry in out:
        sys.stdout.write(entry)
        if not entry.endswith("\n"):
            sys.stdout.write("\n")
with open(after_file,"w") as f:
    f.write(str(last_id))
with open(after_seq_file,"w") as f:
    f.write(str(last_seq))
PY

  if [[ -n "$build_info" ]]; then
    case "$status" in
      built|failed|cached|reused|missing|skipped_known_failure|system_recipe_failed|quarantined|retry)
        echo "Build finished with status: ${status}"
        break
        ;;
    esac
  fi
  sleep "$POLL_INTERVAL"
done
