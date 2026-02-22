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
user_json="$(curl -fsS -X POST "$BASE/v1/admin/users/issue-key" -H "Authorization: Bearer $ADMIN_TOKEN")"
user_key="$(echo "$user_json" | jq -r .api_key)"

tag="smoke-review-$(date +%s)"

echo "== create agents (creator + reviewer) =="
name_a="smoke-agent-a-$(date +%s)"
agent_a_body="$(jq -nc --arg name "$name_a" --arg tag "$tag" '{name:$name,description:"smoke test agent (creator)",tags:["smoke",$tag]}')"
agent_a_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "$agent_a_body")"
agent_a_id="$(echo "$agent_a_json" | jq -r .agent_id)"
agent_a_key="$(echo "$agent_a_json" | jq -r .api_key)"
agent_a_onb_work_item_id="$(echo "$agent_a_json" | jq -r .onboarding.work_item_id)"

name_b="smoke-agent-b-$(date +%s)"
agent_b_body="$(jq -nc --arg name "$name_b" --arg tag "$tag" '{name:$name,description:"smoke test agent (reviewer)",tags:["smoke",$tag,"reviewer"]}')"
agent_b_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "$agent_b_body")"
agent_b_id="$(echo "$agent_b_json" | jq -r .agent_id)"
agent_b_key="$(echo "$agent_b_json" | jq -r .api_key)"

echo "agent_a_id=$agent_a_id"
echo "agent_b_id=$agent_b_id"
echo "agent_a_onboarding_work_item_id=$agent_a_onb_work_item_id"

echo "== poll onboarding offer =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_a_key")"
work_item_id="$(echo "$poll_json" | jq -r --arg wid "$agent_a_onb_work_item_id" '.offers[] | select(.work_item_id==$wid) | .work_item_id' | head -n 1)"
if [[ -z "$work_item_id" ]]; then
  echo "onboarding offer not found in poll" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi

echo "== verify stage_context fields (onboarding) =="
echo "$poll_json" | jq -e --arg wid "$work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.stage_description|type=="string")' >/dev/null
echo "$poll_json" | jq -e --arg wid "$work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.expected_output|type=="object")' >/dev/null
echo "$poll_json" | jq -e --arg wid "$work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.available_skills|type=="array")' >/dev/null
echo "$poll_json" | jq -e --arg wid "$work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.previous_artifacts|type=="array")' >/dev/null

echo "== claim + complete onboarding =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" -H "Authorization: Bearer $agent_a_key" >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/complete" -H "Authorization: Bearer $agent_a_key" >/dev/null

echo "== create run =="
run_body="$(jq -nc --arg tag "$tag" '{goal:"Smoke: write a short paragraph",constraints:"First-person POV. 120-200 words.",required_tags:[$tag]}')"
run_json="$(curl -fsS -X POST "$BASE/v1/runs" \
  -H "Authorization: Bearer $user_key" \
  -H "Content-Type: application/json" \
  -d "$run_body")"
run_id="$(echo "$run_json" | jq -r .run_id)"
echo "run_id=$run_id"

echo "== poll run offer =="
poll2_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_a_key")"
run_work_item_id="$(echo "$poll2_json" | jq -r --arg rid "$run_id" '.offers[] | select(.run_id==$rid and .status=="offered") | .work_item_id' | head -n 1)"
if [[ -z "$run_work_item_id" ]]; then
  echo "run offer not found in poll" >&2
  echo "$poll2_json" | jq . >&2
  exit 1
fi

echo "== verify stage_context fields (run) =="
echo "$poll2_json" | jq -e --arg wid "$run_work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.expected_output.length|type=="string")' >/dev/null
echo "$poll2_json" | jq -e --arg wid "$run_work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.available_skills|type=="array")' >/dev/null
echo "$poll2_json" | jq -e --arg wid "$run_work_item_id" '.offers[] | select(.work_item_id==$wid) | (.stage_context.previous_artifacts|type=="array")' >/dev/null

echo "== claim run work item =="
claim_json="$(curl -fsS -X POST "$BASE/v1/gateway/work-items/$run_work_item_id/claim" -H "Authorization: Bearer $agent_a_key")"
run_id_from_claim="$(echo "$claim_json" | jq -r .run_id)"
if [[ "$run_id_from_claim" != "$run_id" ]]; then
  echo "claim response run_id mismatch: $run_id_from_claim (expected $run_id)" >&2
  exit 1
fi

echo "== emit events =="
ev1="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_a_key" \
  -H "Content-Type: application/json" \
  -d '{"kind":"message","payload":{"text":"starting..."}}')"
ev2="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_a_key" \
  -H "Content-Type: application/json" \
  -d '{"kind":"decision","payload":{"text":"direction A"}}')"
seq2="$(echo "$ev2" | jq -r .seq)"
persona="$(echo "$ev1" | jq -r .persona)"
echo "persona=$persona seq_decision=$seq2"

echo "== submit artifact =="
content="$(printf "This is smoke test output. %.0s" {1..20})"
artifact_body="$(jq -nc --arg content "$content" --argjson seq "$seq2" '{kind:"final",content:$content,linked_event_seq:$seq}')"
art_res="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/artifacts" \
  -H "Authorization: Bearer $agent_a_key" \
  -H "Content-Type: application/json" \
  -d "$artifact_body")"
artifact_id="$(echo "$art_res" | jq -r .artifact_id)"
if [[ -z "$artifact_id" || "$artifact_id" == "null" ]]; then
  echo "artifact_id missing from response" >&2
  echo "$art_res" | jq . >&2
  exit 1
fi

echo "== reviewer polls review work item =="
poll_review="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_b_key")"
review_work_item_id="$(echo "$poll_review" | jq -r --arg rid "$run_id" '.offers[] | select(.run_id==$rid and .kind=="review" and .status=="offered") | .work_item_id' | head -n 1)"
if [[ -z "$review_work_item_id" ]]; then
  echo "review work item not found in poll" >&2
  echo "$poll_review" | jq . >&2
  exit 1
fi
echo "$poll_review" | jq -e --arg wid "$review_work_item_id" --arg aid "$artifact_id" '.offers[] | select(.work_item_id==$wid) | (.review_context.target_artifact_id==$aid and (.review_context.review_criteria|type=="array"))' >/dev/null

echo "== reviewer claims + emits feedback + completes =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$review_work_item_id/claim" -H "Authorization: Bearer $agent_b_key" >/dev/null
review_ev_body="$(jq -nc --arg aid "$artifact_id" '{kind:"summary",payload:{text:"Review: looks good. Consider tightening the ending.",target_artifact_id:$aid}}')"
curl -fsS -X POST "$BASE/v1/gateway/runs/$run_id/events" \
  -H "Authorization: Bearer $agent_b_key" \
  -H "Content-Type: application/json" \
  -d "$review_ev_body" | jq . >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$review_work_item_id/complete" -H "Authorization: Bearer $agent_b_key" >/dev/null

echo "== public checks =="
curl -fsS "$BASE/v1/runs/$run_id" | jq . >/dev/null
curl -fsS "$BASE/v1/runs/$run_id/replay?after_seq=0&limit=200" | jq . >/dev/null
curl -fsS "$BASE/v1/runs/$run_id/output" | jq . >/dev/null

echo "== urls =="
echo "$BASE/ui/"
echo "$BASE/ui/stream.html?run_id=$run_id"
echo "$BASE/ui/replay.html?run_id=$run_id"
echo "$BASE/ui/output.html?run_id=$run_id"
