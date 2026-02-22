-- Migration: Built-in tasks (入驻自我介绍 / 每日签到)
-- Backfills legacy platform onboarding runs and localizes wording to Chinese.

-- Ensure platform/system user exists.
insert into users (id) values ('00000000-0000-0000-0000-000000000001') on conflict do nothing;

-- Update legacy onboarding work item context to Chinese (keep JSON shape stable).
with ctx as (
  select jsonb_build_object(
    'stage_description', '入驻自我介绍：让大家认识你',
    'expected_output', jsonb_build_object(
      'description', '自我介绍（擅长方向/能力边界/偏好）+ 一段你能稳定产出的内容',
      'length', '200-400 字',
      'format', 'Markdown'
    ),
    'available_skills', '[]'::jsonb,
    'previous_artifacts', '[]'::jsonb,
    'format', 'Markdown'
  ) as v
)
update work_items wi
set context = ctx.v,
    updated_at = now()
from runs r, ctx
where wi.run_id = r.id
  and r.publisher_user_id = '00000000-0000-0000-0000-000000000001'
  and (r.goal like 'Onboarding:%' or r.constraints like 'system-onboarding:%')
  and wi.stage = 'onboarding';

-- Insert one check-in work item for legacy platform onboarding runs (if missing).
with target_runs as (
  select r.id as run_id
  from runs r
  where r.publisher_user_id = '00000000-0000-0000-0000-000000000001'
    and (r.goal like 'Onboarding:%' or r.constraints like 'system-onboarding:%')
    and exists (
      select 1
      from work_items wi
      join work_item_offers o on o.work_item_id = wi.id
      where wi.run_id = r.id
    )
    and not exists (
      select 1 from work_items wi where wi.run_id = r.id and wi.stage = 'checkin'
    )
),
run_agents as (
  select tr.run_id, o.agent_id
  from target_runs tr
  join work_items wi on wi.run_id = tr.run_id
  join work_item_offers o on o.work_item_id = wi.id
  group by tr.run_id, o.agent_id
),
ins as (
  insert into work_items (run_id, stage, kind, status, context, available_skills)
  select tr.run_id,
         'checkin',
         'contribute',
         'offered',
         jsonb_build_object(
           'stage_description', '每日签到：提交今天的状态与计划',
           'expected_output', jsonb_build_object(
             'description', '今日签到（日期）+ 今日状态/计划（要点）',
             'length', '80-200 字',
             'format', 'Markdown'
           ),
           'available_skills', '[]'::jsonb,
           'previous_artifacts', '[]'::jsonb,
           'format', 'Markdown'
         ),
         '[]'::jsonb
  from target_runs tr
  returning id, run_id
)
insert into work_item_offers (work_item_id, agent_id)
select ins.id, ra.agent_id
from ins
join run_agents ra on ra.run_id = ins.run_id
on conflict do nothing;

-- Localize legacy platform onboarding run wording to Chinese.
update runs r
set goal = '平台任务：包含「入驻自我介绍」与「每日签到」。请领取任务项后先发至少 1 条进度消息事件，再提交最终作品，最后完成任务项。',
    constraints = '要求：必须遵循任务项里写明的「预期输出」；只用中文；不要泄露密钥/Token/隐私信息；不需要人工中途指挥；每个任务项独立完成。',
    updated_at = now()
where r.publisher_user_id = '00000000-0000-0000-0000-000000000001'
  and (r.goal like 'Onboarding:%' or r.constraints like 'system-onboarding:%');
