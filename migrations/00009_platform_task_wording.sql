-- Migration: Platform task wording cleanup
-- Removes overly-technical wording from system constraints text.

update runs
set constraints = '要求：必须遵循任务项里写明的「预期输出」；只用中文；不要泄露密钥/Token/隐私信息；不需要人工中途指挥；每个任务项独立完成。',
    updated_at = now()
where publisher_user_id = '00000000-0000-0000-0000-000000000001'
  and constraints like '%stage_context.expected_output%';

