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
pub_json="$(curl -fsS -X POST "$BASE/v1/admin/users/issue-key" -H "Authorization: Bearer $ADMIN_TOKEN")"
pub_key="$(echo "$pub_json" | jq -r .api_key)"

echo "== create 3 publisher agents (fill participant slots) =="
mk_agent() {
  local name="$1"
  local body
  body="$(jq -nc --arg name "$name" '{name:$name,description:"publisher agent",tags:["publisher","scifi"]}')"
  curl -fsS -X POST "$BASE/v1/agents" \
    -H "Authorization: Bearer $pub_key" \
    -H "Content-Type: application/json" \
    -d "$body"
}

a1_json="$(mk_agent "pub-agent-1-$(date +%s)")"
a1_key="$(echo "$a1_json" | jq -r .api_key)"
a1_onb_wid="$(echo "$a1_json" | jq -r .onboarding.work_item_id)"
mk_agent "pub-agent-2-$(date +%s)" >/dev/null
mk_agent "pub-agent-3-$(date +%s)" >/dev/null

echo "== complete publisher onboarding (meet publish gate) =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $a1_key")"
onb_wid="$(echo "$poll_json" | jq -r --arg wid "$a1_onb_wid" '.offers[] | select(.work_item_id==$wid) | .work_item_id' | head -n 1)"
if [[ -z "$onb_wid" ]]; then
  echo "publisher onboarding offer not found" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi
curl -fsS -X POST "$BASE/v1/gateway/work-items/$onb_wid/claim" -H "Authorization: Bearer $a1_key" >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$onb_wid/complete" -H "Authorization: Bearer $a1_key" >/dev/null

echo "== create external user + 2 agents (A/B) =="
ext_json="$(curl -fsS -X POST "$BASE/v1/admin/users/issue-key" -H "Authorization: Bearer $ADMIN_TOKEN")"
ext_key="$(echo "$ext_json" | jq -r .api_key)"

mk_ext_agent() {
  local name="$1"
  local body
  body="$(jq -nc --arg name "$name" '{name:$name,description:"external agent",tags:["external","cold-start"]}')"
  curl -fsS -X POST "$BASE/v1/agents" \
    -H "Authorization: Bearer $ext_key" \
    -H "Content-Type: application/json" \
    -d "$body"
}

ext_a_json="$(mk_ext_agent "ext-agent-a-$(date +%s)")"
ext_a_id="$(echo "$ext_a_json" | jq -r .agent_id)"
ext_a_key="$(echo "$ext_a_json" | jq -r .api_key)"
echo "ext_a_id=$ext_a_id"

ext_b_json="$(mk_ext_agent "ext-agent-b-$(date +%s)")"
ext_b_id="$(echo "$ext_b_json" | jq -r .agent_id)"
ext_b_key="$(echo "$ext_b_json" | jq -r .api_key)"
echo "ext_b_id=$ext_b_id"

echo "== publisher creates run (work item offered to publisher agents only) =="
run_body="$(jq -nc '{goal:"Smoke assignment: admin manual assign",constraints:"Verify external agent can be assigned when not matched.",required_tags:["tag-that-does-not-exist"]}')"
run_json="$(curl -fsS -X POST "$BASE/v1/runs" \
  -H "Authorization: Bearer $pub_key" \
  -H "Content-Type: application/json" \
  -d "$run_body")"
run_id="$(echo "$run_json" | jq -r .run_id)"
echo "run_id=$run_id"

echo "== external agents poll: should NOT see run offer yet =="
poll_ext_a="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $ext_a_key")"
found_before_a="$(echo "$poll_ext_a" | jq -r --arg rid "$run_id" '[.offers[] | select(.run_id==$rid)] | length')"
if [[ "$found_before_a" != "0" ]]; then
  echo "unexpected: external agent A already has offer for run" >&2
  echo "$poll_ext_a" | jq . >&2
  exit 1
fi
poll_ext_b="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $ext_b_key")"
found_before_b="$(echo "$poll_ext_b" | jq -r --arg rid "$run_id" '[.offers[] | select(.run_id==$rid)] | length')"
if [[ "$found_before_b" != "0" ]]; then
  echo "unexpected: external agent B already has offer for run" >&2
  echo "$poll_ext_b" | jq . >&2
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

echo "== admin assigns external agent A (add offer) =="
assign_body="$(jq -nc --arg aid "$ext_a_id" '{agent_ids:[$aid], mode:"add", reason:"smoke: manual assign"}')"
curl -fsS -X POST "$BASE/v1/admin/work-items/$work_item_id/assign" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$assign_body" | jq . >/dev/null

echo "== external agent A poll: should see offer now =="
poll_ext_a2="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $ext_a_key")"
found_after_a="$(echo "$poll_ext_a2" | jq -r --arg wid "$work_item_id" '[.offers[] | select(.work_item_id==$wid)] | length')"
if [[ "$found_after_a" != "1" ]]; then
  echo "external agent A did not receive offer after assignment" >&2
  echo "$poll_ext_a2" | jq . >&2
  exit 1
fi

echo "== external agent A claims the work item =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" -H "Authorization: Bearer $ext_a_key" | jq . >/dev/null

echo "== admin force-reassigns to external agent B =="
force_body="$(jq -nc --arg aid "$ext_b_id" '{agent_ids:[$aid], mode:"force_reassign", reason:"smoke: force reassign"}')"
curl -fsS -X POST "$BASE/v1/admin/work-items/$work_item_id/assign" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$force_body" | jq . >/dev/null

echo "== external agent B poll: should see offer now =="
poll_ext_b2="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $ext_b_key")"
found_after_b="$(echo "$poll_ext_b2" | jq -r --arg wid "$work_item_id" '[.offers[] | select(.work_item_id==$wid and .status=="offered")] | length')"
if [[ "$found_after_b" != "1" ]]; then
  echo "external agent B did not receive offer after force-reassign" >&2
  echo "$poll_ext_b2" | jq . >&2
  exit 1
fi

echo "== external agent B claims + completes =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" -H "Authorization: Bearer $ext_b_key" | jq . >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/complete" -H "Authorization: Bearer $ext_b_key" | jq . >/dev/null

echo "== urls =="
echo "$BASE/ui/admin-assign.html"
echo "$BASE/ui/stream.html?run_id=$run_id"
