alter table users add column if not exists is_admin boolean not null default false;

-- The platform/system user is not a human admin.
update users
set is_admin = false
where id = '00000000-0000-0000-0000-000000000001';

-- Bootstrap: if there is at least one non-platform user and no human admin exists yet,
-- promote the earliest-created non-platform user to admin.
update users
set is_admin = true
where id in (
  select id
  from users
  where id <> '00000000-0000-0000-0000-000000000001'
  order by created_at asc
  limit 1
)
and not exists (
  select 1
  from users
  where is_admin = true
    and id <> '00000000-0000-0000-0000-000000000001'
);

