create extension if not exists pgcrypto;

create table if not exists users (
  id uuid primary key default gen_random_uuid(),
  created_at timestamptz not null default now()
);

create table if not exists user_api_keys (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  key_hash text not null,
  created_at timestamptz not null default now(),
  revoked_at timestamptz
);
create unique index if not exists user_api_keys_active_hash on user_api_keys(key_hash) where revoked_at is null;
create index if not exists user_api_keys_user_active on user_api_keys(user_id) where revoked_at is null;

create table if not exists agents (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references users(id) on delete cascade,
  name text not null default '',
  description text not null default '',
  status text not null default 'enabled' check (status in ('enabled', 'disabled')),
  version text not null default 'v1',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists agents_owner_id_idx on agents(owner_id);
create index if not exists agents_status_idx on agents(status);

create table if not exists agent_api_keys (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id) on delete cascade,
  key_hash text not null,
  created_at timestamptz not null default now(),
  revoked_at timestamptz
);
create unique index if not exists agent_api_keys_active_hash on agent_api_keys(key_hash) where revoked_at is null;
create index if not exists agent_api_keys_agent_active on agent_api_keys(agent_id) where revoked_at is null;

create table if not exists agent_tags (
  agent_id uuid not null references agents(id) on delete cascade,
  tag text not null,
  created_at timestamptz not null default now(),
  primary key (agent_id, tag)
);
create index if not exists agent_tags_tag_idx on agent_tags(tag);

create table if not exists runs (
  id uuid primary key default gen_random_uuid(),
  publisher_user_id uuid not null references users(id) on delete cascade,
  goal text not null,
  constraints text not null default '',
  status text not null default 'created' check (status in ('created', 'running', 'completed', 'failed')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists runs_status_idx on runs(status);
create index if not exists runs_created_at_idx on runs(created_at);

create table if not exists work_items (
  id uuid primary key default gen_random_uuid(),
  run_id uuid not null references runs(id) on delete cascade,
  stage text not null,
  kind text not null,
  status text not null default 'offered' check (status in ('offered', 'claimed', 'completed', 'failed')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists work_items_status_idx on work_items(status);
create index if not exists work_items_run_idx on work_items(run_id);

create table if not exists work_item_leases (
  work_item_id uuid primary key references work_items(id) on delete cascade,
  agent_id uuid not null references agents(id) on delete cascade,
  lease_expires_at timestamptz not null,
  created_at timestamptz not null default now()
);
create index if not exists work_item_leases_expires_idx on work_item_leases(lease_expires_at);

create table if not exists events (
  id uuid primary key default gen_random_uuid(),
  run_id uuid not null references runs(id) on delete cascade,
  seq bigint not null,
  kind text not null,
  persona text not null,
  payload jsonb not null default '{}'::jsonb,
  is_key_node boolean not null default false,
  created_at timestamptz not null default now(),
  unique (run_id, seq)
);
create index if not exists events_run_seq_idx on events(run_id, seq);

create table if not exists artifacts (
  id uuid primary key default gen_random_uuid(),
  run_id uuid not null references runs(id) on delete cascade,
  version int not null,
  kind text not null default 'final',
  content text not null,
  linked_event_seq bigint,
  created_at timestamptz not null default now(),
  unique (run_id, version)
);
create index if not exists artifacts_run_version_idx on artifacts(run_id, version);

create table if not exists audit_logs (
  id uuid primary key default gen_random_uuid(),
  actor_type text not null,
  actor_id uuid not null,
  action text not null,
  run_id uuid,
  data jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);
create index if not exists audit_logs_actor_idx on audit_logs(actor_type, actor_id);
create index if not exists audit_logs_run_idx on audit_logs(run_id);

create table if not exists owner_contributions (
  owner_id uuid primary key references users(id) on delete cascade,
  completed_work_items int not null default 0,
  updated_at timestamptz not null default now()
);
