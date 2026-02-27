#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
ADMIN_API_KEY="${ADMIN_API_KEY:-}"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }
}

need curl
need jq

if [[ -z "$ADMIN_API_KEY" ]]; then
  echo "missing ADMIN_API_KEY (admin user api key). Login via /app/admin first to obtain one." >&2
  exit 1
fi

health="$(curl -fsS -m 2 "$BASE/healthz")"
if [[ "$health" != "." ]]; then
  echo "healthz unexpected: $health" >&2
  exit 1
fi

echo "== create user =="
user_json="$(curl -fsS -X POST "$BASE/v1/admin/users/issue-key" -H "Authorization: Bearer $ADMIN_API_KEY")"
user_key="$(echo "$user_json" | jq -r .api_key)"

echo "== create agent =="
name="smoke-mod-agent-$(date +%s)"
agent_body="$(jq -nc --arg name "$name" '{name:$name,description:"moderation smoke",tags:["moderation","safety"]}')"
agent_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "$agent_body")"
agent_key="$(echo "$agent_json" | jq -r .api_key)"
onb_work_item_id="$(echo "$agent_json" | jq -r .onboarding.work_item_id)"

echo "== poll onboarding offer =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
work_item_id="$(echo "$poll_json" | jq -r --arg wid "$onb_work_item_id" '.offers[] | select(.work_item_id==$wid) | .work_item_id' | head -n 1)"
if [[ -z "$work_item_id" ]]; then
  echo "onboarding offer not found in poll" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi

echo "== claim + complete onboarding =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/complete" -H "Authorization: Bearer $agent_key" >/dev/null

marker="SMOKE_MOD_$(date +%s)"

echo "== create run =="
run_body="$(jq -nc --arg marker "$marker" '{goal:("Smoke moderation: " + $marker),constraints:"Contains content to be rejected by admin.",required_tags:["moderation"]}')"
run_json="$(curl -fsS -X POST "$BASE/v1/admin/runs" \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d "$run_body")"
run_id="$(echo "$run_json" | jq -r .run_id)"
echo "run_id=$run_id"

echo "== poll run offer + claim =="
poll2_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
run_work_item_id="$(echo "$poll2_json" | jq -r --arg rid "$run_id" '.offers[] | select(.run_id==$rid and .status=="offered") | .work_item_id' | head -n 1)"
if [[ -z "$run_work_item_id" ]]; then
  echo "run offer not found in poll" >&2
  echo "$poll2_json" | jq . >&2
  exit 1
fi
curl -fsS -X POST "$BASE/v1/gateway/work-items/$run_work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null

echo "== emit event (will be rejected) =="
event_body="$(jq -nc --arg marker "$marker" '{kind:"message",payload:{text:("sensitive fragment " + $marker)}}')"
ev_json="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "$event_body")"
seq="$(echo "$ev_json" | jq -r .seq)"
echo "event_seq=$seq"

echo "== submit artifact (will be rejected) =="
artifact_body="$(jq -nc --arg marker "$marker" --argjson seq "$seq" '{kind:"final",content:("sensitive work " + $marker),linked_event_seq:$seq}')"
art_json="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/artifacts" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "$artifact_body")"
version="$(echo "$art_json" | jq -r .version)"
echo "artifact_version=$version"

echo "== find moderation targets from admin queue =="
event_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=event&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_API_KEY" | jq -r --arg rid "$run_id" --argjson seq "$seq" '.items[] | select(.run_id==$rid and .seq==$seq) | .id' | head -n 1)"
artifact_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=artifact&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_API_KEY" | jq -r --arg rid "$run_id" --argjson v "$version" '.items[] | select(.run_id==$rid and .version==$v) | .id' | head -n 1)"
run_target_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=run&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_API_KEY" | jq -r --arg rid "$run_id" '.items[] | select(.id==$rid) | .id' | head -n 1)"

if [[ -z "$event_id" || -z "$artifact_id" || -z "$run_target_id" ]]; then
  echo "failed to locate targets (event_id=$event_id artifact_id=$artifact_id run_id=$run_target_id)" >&2
  exit 1
fi

echo "== reject event/artifact/run =="
reason_body='{"reason":"smoke: reject"}'
curl -fsS -X POST "$BASE/v1/admin/moderation/event/$event_id/reject" -H "Authorization: Bearer $ADMIN_API_KEY" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
curl -fsS -X POST "$BASE/v1/admin/moderation/artifact/$artifact_id/reject" -H "Authorization: Bearer $ADMIN_API_KEY" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
curl -fsS -X POST "$BASE/v1/admin/moderation/run/$run_target_id/reject" -H "Authorization: Bearer $ADMIN_API_KEY" -H "Content-Type: application/json" -d "$reason_body" >/dev/null

echo "== public checks (no leaks) =="
curl -fsS "$BASE/v1/runs/$run_id/replay?after_seq=0&limit=200" | jq -e --argjson seq "$seq" '.events[] | select(.seq==$seq) | (.payload._redacted==true and .payload.text=="该内容已被管理员审核后屏蔽")' >/dev/null
curl -fsS "$BASE/v1/runs/$run_id/output" | jq -e '.content=="该作品已被管理员审核后屏蔽"' >/dev/null
curl -fsS "$BASE/v1/runs/$run_id" | jq -e '(.goal=="该内容已被管理员审核后屏蔽" and .constraints=="该内容已被管理员审核后屏蔽")' >/dev/null
curl -fsS "$BASE/v1/runs?q=$run_id&limit=10&offset=0" | jq -e '.runs | length == 0' >/dev/null

echo "== urls =="
echo "$BASE/app/admin/moderation"
echo "$BASE/app/runs/$run_id"
