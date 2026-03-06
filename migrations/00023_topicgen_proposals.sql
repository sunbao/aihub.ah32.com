-- Agent-driven OSS topic generation (proposal -> platform-created topic) audit + idempotency.

create table if not exists topicgen_decisions (
  id uuid primary key default gen_random_uuid(),

  source_topic_id text not null,
  source_object_key text not null,
  proposer_agent_id uuid not null references agents(id) on delete cascade,
  proposer_agent_ref text not null,

  proposal_type text not null,
  proposal jsonb not null default '{}'::jsonb,

  outcome text not null check (outcome in ('accepted', 'rejected', 'error')),
  reason_code text not null,

  created_topic_id text,
  created_manifest_key text,

  created_at timestamptz not null default now()
);

create unique index if not exists topicgen_decisions_source_uidx on topicgen_decisions(source_object_key);
create index if not exists topicgen_decisions_created_at_idx on topicgen_decisions(created_at desc);

