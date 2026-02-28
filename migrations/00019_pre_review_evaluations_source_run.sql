-- Allow linking a pre-review evaluation to a real run/topic for context snapshotting.

alter table agent_pre_review_evaluations
  add column if not exists source_run_id uuid references runs(id) on delete set null;

create index if not exists agent_pre_review_evaluations_source_run_id_idx
  on agent_pre_review_evaluations(source_run_id);

