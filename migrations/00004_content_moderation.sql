-- Content moderation (post-review / reversible)

alter table runs add column if not exists review_status text not null default 'pending';
alter table events add column if not exists review_status text not null default 'pending';
alter table artifacts add column if not exists review_status text not null default 'pending';

do $$
begin
  alter table runs add constraint runs_review_status_chk check (review_status in ('pending', 'approved', 'rejected'));
exception when duplicate_object then null;
end $$;

do $$
begin
  alter table events add constraint events_review_status_chk check (review_status in ('pending', 'approved', 'rejected'));
exception when duplicate_object then null;
end $$;

do $$
begin
  alter table artifacts add constraint artifacts_review_status_chk check (review_status in ('pending', 'approved', 'rejected'));
exception when duplicate_object then null;
end $$;

create index if not exists runs_review_status_idx on runs(review_status);
create index if not exists runs_review_created_idx on runs(review_status, created_at);
create index if not exists events_review_status_idx on events(review_status);
create index if not exists events_review_created_idx on events(review_status, created_at);
create index if not exists artifacts_review_status_idx on artifacts(review_status);
create index if not exists artifacts_review_created_idx on artifacts(review_status, created_at);

create table if not exists moderation_actions (
  id uuid primary key default gen_random_uuid(),
  actor_type text not null default 'admin',
  actor_id uuid not null,
  target_type text not null,
  target_id uuid not null,
  action text not null,
  reason text not null default '',
  created_at timestamptz not null default now()
);

do $$
begin
  alter table moderation_actions add constraint moderation_actions_target_type_chk check (target_type in ('run', 'event', 'artifact'));
exception when duplicate_object then null;
end $$;

do $$
begin
  alter table moderation_actions add constraint moderation_actions_action_chk check (action in ('approve', 'reject', 'unreject'));
exception when duplicate_object then null;
end $$;

create index if not exists moderation_actions_target_idx on moderation_actions(target_type, target_id);
create index if not exists moderation_actions_created_at_idx on moderation_actions(created_at);
