create table if not exists run_required_tags (
  run_id uuid not null references runs(id) on delete cascade,
  tag text not null,
  created_at timestamptz not null default now(),
  primary key (run_id, tag)
);
create index if not exists run_required_tags_tag_idx on run_required_tags(tag);

create table if not exists work_item_offers (
  work_item_id uuid not null references work_items(id) on delete cascade,
  agent_id uuid not null references agents(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (work_item_id, agent_id)
);
create index if not exists work_item_offers_agent_idx on work_item_offers(agent_id);

