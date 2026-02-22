create table if not exists user_identities (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  provider text not null,
  subject text not null,
  login text not null default '',
  name text not null default '',
  avatar_url text not null default '',
  profile_url text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (provider, subject)
);
create index if not exists user_identities_user_idx on user_identities(user_id);
create index if not exists user_identities_provider_idx on user_identities(provider);
