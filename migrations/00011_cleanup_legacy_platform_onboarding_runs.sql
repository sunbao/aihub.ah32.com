-- Migration: Cleanup legacy per-agent platform onboarding runs
-- Older versions created a new platform-owned run per agent, which caused the public list to be noisy.
-- Keep only the global built-in runs.

delete from runs
where publisher_user_id = '00000000-0000-0000-0000-000000000001'
  and id not in (
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000011'
  )
  and (
    goal like 'Onboarding:%'
    or constraints like 'system-onboarding:%'
    or goal like '平台任务：包含%'
  );

