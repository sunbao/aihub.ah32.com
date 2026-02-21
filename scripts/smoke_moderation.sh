#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-change-me-admin}"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }
}

need curl
need jq

health="$(curl -fsS -m 2 "$BASE/healthz")"
if [[ "$health" != "." ]]; then
  echo "healthz unexpected: $health" >&2
  exit 1
fi

echo "== create user =="
user_json="$(curl -fsS -X POST "$BASE/v1/users")"
user_key="$(echo "$user_json" | jq -r .api_key)"

echo "== create agent =="
name="smoke-mod-agent-$(date +%s)"
agent_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$name\",\"description\":\"moderation smoke\",\"tags\":[\"内容审核\",\"安全\"]}")"
agent_key="$(echo "$agent_json" | jq -r .api_key)"
onb_work_item_id="$(echo "$agent_json" | jq -r .onboarding.work_item_id)"

echo "== poll onboarding offer =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
work_item_id="$(echo "$poll_json" | jq -r --arg wid "$onb_work_item_id" ".offers[] | select(.work_item_id==\$wid) | .work_item_id" | head -n 1)"
if [[ -z "$work_item_id" ]]; then
  echo "onboarding offer not found in poll" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi

echo "== claim + complete onboarding =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/complete" -H "Authorization: Bearer $agent_key" >/dev/null

echo "== create run =="
run_json="$(curl -fsS -X POST "$BASE/v1/runs" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "{\"goal\":\"冒烟测试：内容审核\",\"constraints\":\"包含敏感片段以便管理员屏蔽\",\"required_tags\":[\"内容审核\"]}")"
run_id="$(echo "$run_json" | jq -r .run_id)"
echo "run_id=$run_id"

echo "== poll run offer + claim =="
poll2_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
run_work_item_id="$(echo "$poll2_json" | jq -r --arg rid "$run_id" ".offers[] | select(.run_id==\$rid) | .work_item_id" | head -n 1)"
if [[ -z "$run_work_item_id" ]]; then
  echo "run offer not found in poll" >&2
  echo "$poll2_json" | jq . >&2
  exit 1
fi
curl -fsS -X POST "$BASE/v1/gateway/work-items/$run_work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null

echo "== emit event (will be rejected) =="
marker="SMOKE_MOD_$(date +%s)"
ev_json="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"message\",\"payload\":{\"text\":\"敏感片段：$marker\"}}")"
seq="$(echo "$ev_json" | jq -r .seq)"
echo "event_seq=$seq"

echo "== submit artifact (will be rejected) =="
art_json="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/artifacts" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"final\",\"content\":\"敏感作品：$marker\",\"linked_event_seq\":$seq}")"
version="$(echo "$art_json" | jq -r .version)"
echo "artifact_version=$version"

echo "== find moderation targets from admin queue =="
event_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=event&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r --arg rid "$run_id" --argjson seq "$seq" '.items[] | select(.run_id==$rid and .seq==$seq) | .id' | head -n 1)"
artifact_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=artifact&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r --arg rid "$run_id" --argjson v "$version" '.items[] | select(.run_id==$rid and .version==$v) | .id' | head -n 1)"
run_target_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=run&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r --arg rid "$run_id" '.items[] | select(.id==$rid) | .id' | head -n 1)"

if [[ -z "$event_id" || -z "$artifact_id" || -z "$run_target_id" ]]; then
  echo "failed to locate targets (event_id=$event_id artifact_id=$artifact_id run_id=$run_target_id)" >&2
  exit 1
fi

echo "== reject event/artifact/run =="
reason_body='{"reason":"smoke: reject"}'
curl -fsS -X POST "$BASE/v1/admin/moderation/event/$event_id/reject" -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
curl -fsS -X POST "$BASE/v1/admin/moderation/artifact/$artifact_id/reject" -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
curl -fsS -X POST "$BASE/v1/admin/moderation/run/$run_target_id/reject" -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d "$reason_body" >/dev/null

echo "== public checks (no leaks) =="
curl -fsS "$BASE/v1/runs/$run_id/replay?after_seq=0&limit=200" | jq -e --argjson seq "$seq" '.events[] | select(.seq==$seq) | (.payload._redacted==true and .payload.text=="该内容已被管理员审核后屏蔽")' >/dev/null
curl -fsS "$BASE/v1/runs/$run_id/output" | jq -e '.content=="该作品已被管理员审核后屏蔽"' >/dev/null
curl -fsS "$BASE/v1/runs/$run_id" | jq -e '(.goal=="该内容已被管理员审核后屏蔽" and .constraints=="该内容已被管理员审核后屏蔽")' >/dev/null
curl -fsS "$BASE/v1/runs?q=$run_id&limit=10&offset=0" | jq -e '.runs | length == 0' >/dev/null

echo "== urls =="
echo "$BASE/ui/admin.html"
echo "$BASE/ui/stream.html?run_id=$run_id"
echo "$BASE/ui/replay.html?run_id=$run_id"
echo "$BASE/ui/output.html?run_id=$run_id"

