#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
ADMIN_API_KEY="${ADMIN_API_KEY:-}"
RUN_OFFER_POLL_MAX="${RUN_OFFER_POLL_MAX:-20}"
RUN_OFFER_POLL_INTERVAL_SEC="${RUN_OFFER_POLL_INTERVAL_SEC:-1}"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing dependency: $1" >&2; exit 1; }
}

need curl
need jq

if [[ -z "$ADMIN_API_KEY" ]]; then
  echo "missing ADMIN_API_KEY (admin user api key). Login via /app/admin first to obtain one." >&2
  exit 1
fi

# Production hygiene: always clean up smoke data unless explicitly kept.
KEEP_SMOKE_DATA="${KEEP_SMOKE_DATA:-}"
agent_ref=""
run_ref=""
cleanup() {
  status="$?"
  if [[ -n "$KEEP_SMOKE_DATA" ]]; then
    exit "$status"
  fi

  cleanup_ok=1
  if [[ -n "${run_ref:-}" ]]; then
    if ! curl -fsS -X DELETE "$BASE/v1/admin/runs/$run_ref" -H "Authorization: Bearer $ADMIN_API_KEY" >/dev/null; then
      echo "cleanup failed: delete run run_ref=$run_ref" >&2
      cleanup_ok=0
    fi
  fi
  if [[ -n "${agent_ref:-}" ]]; then
    if ! curl -fsS -X DELETE "$BASE/v1/agents/$agent_ref" -H "Authorization: Bearer $ADMIN_API_KEY" >/dev/null; then
      echo "cleanup failed: delete agent agent_ref=$agent_ref" >&2
      cleanup_ok=0
    fi
  fi

  if [[ "$cleanup_ok" -eq 0 ]]; then
    exit 1
  fi
  exit "$status"
}
trap cleanup EXIT

health="$(curl -fsS -m 2 "$BASE/healthz")"
if [[ "$health" != "." ]]; then
  echo "healthz unexpected: $health" >&2
  exit 1
fi

echo "== owner context =="
echo "skip: not creating extra user; use admin owner for deterministic matching"
marker="SMOKE_MOD_$(date +%s)"

echo "== create agent =="
name="smoke-mod-agent-$(date +%s)"
agent_body="$(jq -nc --arg name "$name" --arg marker "$marker" '{name:$name,description:"moderation smoke",tags:["moderation","safety",$marker]}')"
agent_json="$(curl -fsS -X POST "$BASE/v1/agents" \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d "$agent_body")"
agent_ref="$(echo "$agent_json" | jq -r .agent_ref)"
agent_key="$(echo "$agent_json" | jq -r .api_key)"
echo "agent_ref=$agent_ref"

echo "== poll onboarding offer =="
poll_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
work_item_id="$(echo "$poll_json" | jq -r --arg aref "$agent_ref" '.offers[] | select(.stage=="onboarding" and .status=="offered" and .stage_context.self_agent_ref==$aref) | .work_item_id' | head -n 1)"
if [[ -z "$work_item_id" ]]; then
  echo "onboarding offer not found in poll" >&2
  echo "$poll_json" | jq . >&2
  exit 1
fi

echo "== claim + complete onboarding =="
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null
curl -fsS -X POST "$BASE/v1/gateway/work-items/$work_item_id/complete" -H "Authorization: Bearer $agent_key" >/dev/null

echo "== create run =="
run_body="$(jq -nc --arg marker "$marker" '{goal:("Smoke moderation: " + $marker),constraints:"Contains content to be rejected by admin.",required_tags:[$marker]}')"
run_json="$(curl -fsS -X POST "$BASE/v1/admin/runs" \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d "$run_body")"
run_ref="$(echo "$run_json" | jq -r .run_ref)"
echo "run_ref=$run_ref"

echo "== poll run offer + claim =="
poll2_json=""
run_work_item_id=""
for attempt in $(seq 1 "$RUN_OFFER_POLL_MAX"); do
  poll2_json="$(curl -fsS "$BASE/v1/gateway/inbox/poll" -H "Authorization: Bearer $agent_key")"
  run_work_item_id="$(echo "$poll2_json" | jq -r --arg rref "$run_ref" '.offers[] | select(.run_ref==$rref and .status=="offered") | .work_item_id' | head -n 1)"
  if [[ -n "$run_work_item_id" ]]; then
    break
  fi

  other_work_item_id="$(echo "$poll2_json" | jq -r --arg rref "$run_ref" '.offers[] | select((.run_ref // "") != $rref and .status=="offered") | .work_item_id' | head -n 1)"
  if [[ -n "$other_work_item_id" ]]; then
    other_stage="$(echo "$poll2_json" | jq -r --arg wid "$other_work_item_id" '.offers[] | select(.work_item_id==$wid) | .stage' | head -n 1)"
    other_run_ref="$(echo "$poll2_json" | jq -r --arg wid "$other_work_item_id" '.offers[] | select(.work_item_id==$wid) | .run_ref' | head -n 1)"
    echo "drain non-target offer attempt=$attempt stage=$other_stage run_ref=$other_run_ref work_item_id=$other_work_item_id"
    curl -fsS -X POST "$BASE/v1/gateway/work-items/$other_work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null
    curl -fsS -X POST "$BASE/v1/gateway/work-items/$other_work_item_id/complete" -H "Authorization: Bearer $agent_key" >/dev/null
  else
    echo "run offer not ready yet attempt=$attempt/$RUN_OFFER_POLL_MAX"
  fi
  sleep "$RUN_OFFER_POLL_INTERVAL_SEC"
done
if [[ -z "$run_work_item_id" ]]; then
  echo "run offer not found in poll" >&2
  echo "$poll2_json" | jq . >&2
  exit 1
fi
curl -fsS -X POST "$BASE/v1/gateway/work-items/$run_work_item_id/claim" -H "Authorization: Bearer $agent_key" >/dev/null

echo "== emit event (will be rejected) =="
event_body="$(jq -nc --arg marker "$marker" '{kind:"message",payload:{text:("sensitive fragment " + $marker)}}')"
ev_json="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_ref/events" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "$event_body")"
seq="$(echo "$ev_json" | jq -r .seq)"
echo "event_seq=$seq"

echo "== submit artifact (will be rejected) =="
artifact_body="$(jq -nc --arg marker "$marker" --argjson seq "$seq" '{kind:"final",content:("sensitive work " + $marker),linked_event_seq:$seq}')"
art_json="$(curl -fsS -X POST "$BASE/v1/gateway/runs/$run_ref/artifacts" \
  -H "Authorization: Bearer $agent_key" \
  -H "Content-Type: application/json" \
  -d "$artifact_body")"
version="$(echo "$art_json" | jq -r .version)"
echo "artifact_version=$version"

echo "== find moderation targets from admin queue =="
event_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=event&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_API_KEY" | jq -r --arg rref "$run_ref" --argjson seq "$seq" '(.items // [])[] | select(.run_ref==$rref and .seq==$seq) | .id' | head -n 1)"
artifact_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=artifact&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_API_KEY" | jq -r --arg rref "$run_ref" --argjson v "$version" '(.items // [])[] | select(.run_ref==$rref and .version==$v) | .id' | head -n 1)"
run_target_id="$(curl -fsS "$BASE/v1/admin/moderation/queue?status=pending&types=run&limit=200&offset=0" \
  -H "Authorization: Bearer $ADMIN_API_KEY" | jq -r --arg rref "$run_ref" '(.items // [])[] | select(.run_ref==$rref) | .id' | head -n 1)"

if [[ -z "$event_id" || -z "$artifact_id" ]]; then
  echo "failed to locate targets (event_id=$event_id artifact_id=$artifact_id run_target_id=$run_target_id)" >&2
  exit 1
fi

echo "== reject event/artifact/run =="
reason_body='{"reason":"smoke: reject"}'
curl -fsS -X POST "$BASE/v1/admin/moderation/event/$event_id/reject" -H "Authorization: Bearer $ADMIN_API_KEY" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
curl -fsS -X POST "$BASE/v1/admin/moderation/artifact/$artifact_id/reject" -H "Authorization: Bearer $ADMIN_API_KEY" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
if [[ -n "$run_target_id" ]]; then
  curl -fsS -X POST "$BASE/v1/admin/moderation/run/$run_target_id/reject" -H "Authorization: Bearer $ADMIN_API_KEY" -H "Content-Type: application/json" -d "$reason_body" >/dev/null
else
  echo "skip run-level rejection: no pending run moderation target in queue"
fi

echo "== public checks (no leaks) =="
curl -fsS "$BASE/v1/runs/$run_ref/replay?after_seq=0&limit=200" | jq -e --argjson seq "$seq" '.events[] | select(.seq==$seq) | (.payload._redacted==true)' >/dev/null
curl -fsS "$BASE/v1/runs/$run_ref/output" | jq -e --arg marker "$marker" '.content | contains($marker) | not' >/dev/null
if [[ -n "$run_target_id" ]]; then
  curl -fsS "$BASE/v1/runs/$run_ref" | jq -e --arg marker "$marker" '((.goal | contains($marker) | not) and (.constraints | contains($marker) | not))' >/dev/null
  curl -fsS "$BASE/v1/runs?q=$run_ref&limit=10&offset=0" | jq -e '.runs | length == 0' >/dev/null
else
  echo "skip run-level public checks: no run moderation target in queue"
fi

echo "== urls =="
echo "$BASE/app/admin/moderation"
echo "$BASE/app/runs/$run_ref"

echo "SMOKE_MOD_META agent_ref=$agent_ref run_ref=$run_ref marker=$marker"
