#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"

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
user_id="$(echo "$user_json" | jq -r .user_id)"
user_key="$(echo "$user_json" | jq -r .api_key)"
echo "user_id=$user_id"

echo "== create agent =="
name="smoke-agent-$(date +%s)"
agent_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$name\",\"description\":\"smoke test agent\",\"tags\":[\"科幻创意\",\"逻辑校对\"]}")"
agent_id="$(echo "$agent_json" | jq -r .agent_id)"
agent_key="$(echo "$agent_json" | jq -r .api_key)"
onb_run_id="$(echo "$agent_json" | jq -r .onboarding.run_id)"
onb_work_item_id="$(echo "$agent_json" | jq -r .onboarding.work_item_id)"
echo "agent_id=$agent_id"
echo "onboarding_run_id=$onb_run_id"
echo "onboarding_work_item_id=$onb_work_item_id"

echo "== poll onboarding offer =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
work_item_id="$(echo "$poll_json" | jq -r --arg wid "$onb_work_item_id" ".offers[] | select(.work_item_id==\$wid) | .work_item_id" | head -n 1)"
if [[ -z "$work_item_id" ]]; then
  echo "onboarding offer not found in poll" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi

echo "== claim onboarding =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" \
  -H "Authorization: Bearer $agent_key" | jq .

echo "== complete onboarding =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/complete" \
  -H "Authorization: Bearer $agent_key" | jq .

echo "== create run =="
run_json="$(curl -fsS -X POST "$BASE/v1/runs" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "{\"goal\":\"冒烟测试：写 200 字短文\",\"constraints\":\"第一人称\",\"required_tags\":[\"科幻创意\"]}")"
run_id="$(echo "$run_json" | jq -r .run_id)"
echo "run_id=$run_id"

echo "== poll run offer =="
poll2_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
run_work_item_id="$(echo "$poll2_json" | jq -r --arg rid "$run_id" ".offers[] | select(.run_id==\$rid and .status==\"offered\") | .work_item_id" | head -n 1)"
if [[ -z "$run_work_item_id" ]]; then
  run_work_item_id="$(echo "$poll2_json" | jq -r --arg rid "$run_id" ".offers[] | select(.run_id==\$rid) | .work_item_id" | head -n 1)"
fi
if [[ -z "$run_work_item_id" ]]; then
  echo "run offer not found in poll" >&2
  echo "$poll2_json" | jq . >&2
  exit 1
fi

echo "== claim run work item =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$run_work_item_id/claim" \
  -H "Authorization: Bearer $agent_key" | jq .

echo "== emit events =="
ev1="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"message\",\"payload\":{\"text\":\"开始构思...\"}}")"
ev2="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"decision\",\"payload\":{\"text\":\"选用方向 A\"}}")"
seq2="$(echo "$ev2" | jq -r .seq)"
persona="$(echo "$ev1" | jq -r .persona)"
echo "persona=$persona seq_decision=$seq2"

echo "== submit artifact =="
content="$(printf "这是冒烟测试的最终作品。%.0s" {1..20})"
curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/artifacts" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"final\",\"content\":\"$content\",\"linked_event_seq\":$seq2}" | jq .

echo "== public checks =="
curl -fsS "$BASE/v1/runs/$run_id" | jq . >/dev/null
curl -fsS "$BASE/v1/runs/$run_id/replay?after_seq=0&limit=200" | jq . >/dev/null
curl -fsS "$BASE/v1/runs/$run_id/output" | jq . >/dev/null

echo "== urls =="
echo "$BASE/ui/"
echo "$BASE/ui/stream.html?run_id=$run_id"
echo "$BASE/ui/replay.html?run_id=$run_id"
echo "$BASE/ui/output.html?run_id=$run_id"
