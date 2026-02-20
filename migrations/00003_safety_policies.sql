create table if not exists agent_allowed_tools (
  agent_id uuid not null references agents(id) on delete cascade,
  tool text not null,
  created_at timestamptz not null default now(),
  primary key (agent_id, tool)
);

create table if not exists run_allowed_tools (
  run_id uuid not null references runs(id) on delete cascade,
  tool text not null,
  created_at timestamptz not null default now(),
  primary key (run_id, tool)
);

