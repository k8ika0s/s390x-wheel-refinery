#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

API_BASE="${API_BASE:-http://localhost:8080}"
PACKAGE="${PACKAGE:-six}"
VERSION="${VERSION:-1.16.0}"
REQ_LINE="${REQ_LINE:-}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-180}"
POLL_INTERVAL="${POLL_INTERVAL:-2}"

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
  rm -f "$tmp_req" "${after_file:-}"
}
trap cleanup EXIT

printf "%s\n" "$REQ_LINE" > "$tmp_req"

echo "Uploading requirements: ${REQ_LINE}"
upload_resp="$(curl -sS -f -X POST -F "file=@${tmp_req};filename=requirements-seed.txt" "${API_BASE}/api/requirements/upload")"
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

if ! curl -sS -f -X POST "${API_BASE}/api/pending-inputs/${pending_id}/enqueue-plan" >/dev/null; then
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
enqueue_resp="$(curl -sS -f -X POST "${API_BASE}/api/plans/${plan_id}/enqueue-builds")"
echo "Builds enqueued: ${enqueue_resp}"

if [[ -n "${WORKER_TOKEN:-}" ]]; then
  curl -sS -f -X POST -H "X-Worker-Token: ${WORKER_TOKEN}" "${API_BASE}/api/worker/trigger" >/dev/null || true
fi

after_file="$(mktemp)"
echo "Tailing logs for ${PACKAGE} ${VERSION}..."
prev_status=""

while true; do
  curl -sS "${API_BASE}/api/logs/chunks/${PACKAGE}/${VERSION}?after=$(cat "$after_file" 2>/dev/null || echo 0)&limit=200" \
    | python3 - "$after_file" <<'PY'
import json,sys
after_file=sys.argv[1]
data=json.load(sys.stdin)
last=0
out=[]
for chunk in data:
    if not chunk:
        continue
    content=chunk.get("content") or ""
    if content:
        out.append(content)
    cid=chunk.get("id")
    if cid:
        last=cid
if out:
    for entry in out:
        sys.stdout.write(entry)
        if not entry.endswith("\n"):
            sys.stdout.write("\n")
with open(after_file,"w") as f:
    f.write(str(last))
PY

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
    case "$status" in
      built|failed|cached|reused|missing|skipped_known_failure|system_recipe_failed)
        echo "Build finished with status: ${status}"
        break
        ;;
    esac
  else
    echo "Waiting for build record..."
  fi
  sleep "$POLL_INTERVAL"
done
