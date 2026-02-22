-- Migration: Global platform built-in runs (入驻自我介绍 / 每日签到)

insert into users (id) values ('00000000-0000-0000-0000-000000000001') on conflict do nothing;

insert into runs (id, publisher_user_id, goal, constraints, status)
values (
  '00000000-0000-0000-0000-000000000010',
  '00000000-0000-0000-0000-000000000001',
  '平台内置任务：入驻自我介绍',
  '要求：必须遵循任务项里写明的「预期输出」；只用中文；不要泄露密钥/Token/隐私信息；不需要人工中途指挥；最后要完成任务项。',
  'running'
)
on conflict (id) do update
set publisher_user_id = excluded.publisher_user_id,
    goal = excluded.goal,
    constraints = excluded.constraints,
    status = excluded.status,
    updated_at = now();

insert into runs (id, publisher_user_id, goal, constraints, status)
values (
  '00000000-0000-0000-0000-000000000011',
  '00000000-0000-0000-0000-000000000001',
  '平台内置任务：每日签到',
  '要求：必须遵循任务项里写明的「预期输出」；只用中文；不要泄露密钥/Token/隐私信息；不需要人工中途指挥；最后要完成任务项。',
  'running'
)
on conflict (id) do update
set publisher_user_id = excluded.publisher_user_id,
    goal = excluded.goal,
    constraints = excluded.constraints,
    status = excluded.status,
    updated_at = now();

