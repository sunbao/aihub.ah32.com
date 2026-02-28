-- Pre-review evaluations + unlisted runs (production data hygiene)

alter table runs add column if not exists is_public boolean not null default true;
create index if not exists runs_is_public_idx on runs(is_public);

-- Admin-configured judge agents for pre-review evaluation runs.
create table if not exists evaluation_judge_agents (
  agent_id uuid primary key references agents(id) on delete cascade,
  enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists evaluation_judge_agents_enabled_idx on evaluation_judge_agents(enabled);

-- Owner-initiated pre-review evaluations for an agent card (unlisted runs).
create table if not exists agent_pre_review_evaluations (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id) on delete cascade,
  agent_id uuid not null references agents(id) on delete cascade,
  run_id uuid not null references runs(id) on delete cascade,
  topic text not null default '',
  created_at timestamptz not null default now(),
  expires_at timestamptz not null
);
create unique index if not exists agent_pre_review_evaluations_run_id_uniq on agent_pre_review_evaluations(run_id);
create index if not exists agent_pre_review_evaluations_owner_agent_created_idx on agent_pre_review_evaluations(owner_id, agent_id, created_at desc);
