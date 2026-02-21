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

echo "== create publisher user =="
pub_json="$(curl -fsS -X POST "$BASE/v1/users")"
pub_key="$(echo "$pub_json" | jq -r .api_key)"

echo "== create 3 publisher agents (fill participant slots) =="
mk_agent() {
  local name="$1"
  curl -fsS -X POST "$BASE/v1/agents" \
    -H "Authorization: Bearer $pub_key" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$name\",\"description\":\"publisher agent\",\"tags\":[\"创作\",\"科幻创意\"]}"
}

a1_json="$(mk_agent "pub-agent-1-$(date +%s)")"
a1_key="$(echo "$a1_json" | jq -r .api_key)"
a1_onb_wid="$(echo "$a1_json" | jq -r .onboarding.work_item_id)"
mk_agent "pub-agent-2-$(date +%s)" >/dev/null
mk_agent "pub-agent-3-$(date +%s)" >/dev/null

echo "== complete publisher onboarding (meet publish gate) =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $a1_key")"
onb_wid="$(echo "$poll_json" | jq -r --arg wid "$a1_onb_wid" ".offers[] | select(.work_item_id==\$wid) | .work_item_id" | head -n 1)"
if [[ -z "$onb_wid" ]]; then
  echo "publisher onboarding offer not found" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi
curl -fsS -X POST "$BASE/v1/gateway/work-items/$onb_wid/claim" -H "Authorization: Bearer $a1_key" >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$onb_wid/complete" -H "Authorization: Bearer $a1_key" >/dev/null

echo "== create external user + agent (will be manually assigned) =="
ext_json="$(curl -fsS -X POST "$BASE/v1/users")"
ext_key="$(echo "$ext_json" | jq -r .api_key)"
ext_agent_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $ext_key" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"ext-agent-$(date +%s)\",\"description\":\"external agent\",\"tags\":[\"外部\",\"冷启动\"]}")"
ext_agent_id="$(echo "$ext_agent_json" | jq -r .agent_id)"
ext_agent_key="$(echo "$ext_agent_json" | jq -r .api_key)"
echo "ext_agent_id=$ext_agent_id"

echo "== publisher creates run (work item offered to publisher agents only) =="
run_json="$(curl -fsS -X POST "$BASE/v1/runs" \
  -H "Authorization: Bearer $pub_key" \
  -H "Content-Type: application/json" \
  -d "{\"goal\":\"冒烟测试：管理员手工指派\",\"constraints\":\"验证外部 agent 在未匹配时也能被指派\",\"required_tags\":[\"绝对不存在标签\"]}")"
run_id="$(echo "$run_json" | jq -r .run_id)"
echo "run_id=$run_id"

echo "== external agent poll: should NOT see run offer yet =="
poll_ext="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $ext_agent_key")"
found_before="$(echo "$poll_ext" | jq -r --arg rid "$run_id" '[.offers[] | select(.run_id==$rid)] | length')"
if [[ "$found_before" != "0" ]]; then
  echo "unexpected: external agent already has offer for run" >&2
  echo "$poll_ext" | jq . >&2
  exit 1
fi

echo "== admin locates work item via admin API =="
wi_json="$(curl -fsS "$BASE/v1/admin/work-items?run_id=$run_id&limit=20&offset=0" -H "Authorization: Bearer $ADMIN_TOKEN")"
work_item_id="$(echo "$wi_json" | jq -r '.items[0].work_item_id')"
if [[ -z "$work_item_id" || "$work_item_id" == "null" ]]; then
  echo "failed to find work item for run via admin API" >&2
  echo "$wi_json" | jq . >&2
  exit 1
fi
echo "work_item_id=$work_item_id"

echo "== admin assigns external agent (add offer) =="
assign_body="$(jq -nc --arg aid "$ext_agent_id" '{agent_ids:[$aid], mode:"add", reason:"smoke: manual assign"}')"
curl -fsS -X POST "$BASE/v1/admin/work-items/$work_item_id/assign" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$assign_body" | jq .

echo "== external agent poll: should see offer now =="
poll_ext2="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $ext_agent_key")"
found_after="$(echo "$poll_ext2" | jq -r --arg wid "$work_item_id" '[.offers[] | select(.work_item_id==$wid)] | length')"
if [[ "$found_after" != "1" ]]; then
  echo "external agent did not receive offer after assignment" >&2
  echo "$poll_ext2" | jq . >&2
  exit 1
fi

echo "== urls =="
echo "$BASE/ui/admin-assign.html"
echo "$BASE/ui/stream.html?run_id=$run_id"
