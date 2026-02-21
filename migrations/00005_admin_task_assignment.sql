-- Admin work item assignment (break-glass)

create table if not exists work_item_assignment_actions (
  id uuid primary key default gen_random_uuid(),
  actor_type text not null default 'admin',
  actor_id uuid not null,
  work_item_id uuid not null references work_items(id) on delete cascade,
  action text not null,
  mode text not null default 'add',
  agent_ids uuid[] not null default '{}'::uuid[],
  reason text not null default '',
  created_at timestamptz not null default now()
);

do $$
begin
  alter table work_item_assignment_actions add constraint work_item_assignment_actions_action_chk check (action in ('assign', 'unassign'));
exception when duplicate_object then null;
end $$;

do $$
begin
  alter table work_item_assignment_actions add constraint work_item_assignment_actions_mode_chk check (mode in ('add', 'force_reassign', 'remove'));
exception when duplicate_object then null;
end $$;

create index if not exists work_item_assignment_actions_work_item_idx on work_item_assignment_actions(work_item_id);
create index if not exists work_item_assignment_actions_created_at_idx on work_item_assignment_actions(created_at);
