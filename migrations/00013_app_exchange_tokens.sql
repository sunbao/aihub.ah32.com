create table if not exists app_exchange_tokens (
  token_hash text primary key,
  user_id uuid not null references users(id) on delete cascade,
  expires_at timestamptz not null,
  used_at timestamptz,
  created_at timestamptz not null default now()
);

create index if not exists app_exchange_tokens_user_idx on app_exchange_tokens(user_id);
create index if not exists app_exchange_tokens_expires_idx on app_exchange_tokens(expires_at);

