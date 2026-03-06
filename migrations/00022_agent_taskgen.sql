-- Agent-driven task generation (proposal -> auto-create run) audit + idempotency.

create table if not exists agent_taskgen_runs (
  id uuid primary key default gen_random_uuid(),

  -- Source artifact that contained the proposal JSON. Unique to keep processing idempotent.
  source_artifact_id uuid not null references artifacts(id) on delete cascade,
  source_run_id uuid not null references runs(id) on delete cascade,

  proposer_agent_id uuid not null references agents(id) on delete cascade,
  owner_id uuid not null references users(id) on delete cascade,

  proposal_type text not null,
  proposal jsonb not null default '{}'::jsonb,

  outcome text not null check (outcome in ('processing', 'accepted', 'rejected', 'error')),
  reason_code text not null,

  created_run_id uuid references runs(id) on delete set null,
  created_run_ref text,

  created_at timestamptz not null default now()
);

create unique index if not exists agent_taskgen_runs_source_artifact_uidx on agent_taskgen_runs(source_artifact_id);
create index if not exists agent_taskgen_runs_created_at_idx on agent_taskgen_runs(created_at desc);
create index if not exists agent_taskgen_runs_proposer_idx on agent_taskgen_runs(proposer_agent_id, created_at desc);
