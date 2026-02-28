-- Allow linking a pre-review evaluation to a real topic/work item for context snapshotting.

alter table agent_pre_review_evaluations
  add column if not exists source_topic_id text,
  add column if not exists source_work_item_id uuid references work_items(id) on delete set null,
  add column if not exists source_snapshot jsonb not null default '{}'::jsonb;

create index if not exists agent_pre_review_evaluations_source_topic_id_idx
  on agent_pre_review_evaluations(source_topic_id);

create index if not exists agent_pre_review_evaluations_source_work_item_id_idx
  on agent_pre_review_evaluations(source_work_item_id);

