-- Agent Home 32: Agent Card fields + OSS registry primitives

-- Agent Card fields (stored on agents for MVP; prompts/certs are derived)
alter table agents add column if not exists personality jsonb not null default '{}'::jsonb;
alter table agents add column if not exists interests jsonb not null default '[]'::jsonb;
alter table agents add column if not exists capabilities jsonb not null default '[]'::jsonb;
alter table agents add column if not exists bio text not null default '';
alter table agents add column if not exists greeting text not null default '';
alter table agents add column if not exists avatar_url text not null default '';
alter table agents add column if not exists discovery jsonb not null default '{}'::jsonb;
alter table agents add column if not exists autonomous jsonb not null default '{}'::jsonb;
alter table agents add column if not exists persona jsonb;

-- Admission / identity keys
alter table agents add column if not exists agent_public_key text not null default '';
alter table agents add column if not exists admitted_status text not null default 'not_requested';
alter table agents add column if not exists admitted_at timestamptz;

do $$
begin
  alter table agents add constraint agents_admitted_status_chk check (admitted_status in ('not_requested','pending','admitted','rejected'));
exception when duplicate_object then null;
end $$;

-- Platform-certified derived views
alter table agents add column if not exists card_version int not null default 1;
alter table agents add column if not exists prompt_view text not null default '';
alter table agents add column if not exists card_cert jsonb not null default '{}'::jsonb;
alter table agents add column if not exists card_review_status text not null default 'approved';

do $$
begin
  alter table agents add constraint agents_card_review_status_chk check (card_review_status in ('pending','approved','rejected'));
exception when duplicate_object then null;
end $$;

create index if not exists agents_admitted_status_idx on agents(admitted_status);
create index if not exists agents_card_review_status_idx on agents(card_review_status);

-- Admission challenges (challenge/response PoP)
create table if not exists agent_admission_challenges (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id) on delete cascade,
  challenge text not null,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  consumed_at timestamptz
);
create index if not exists agent_admission_challenges_agent_idx on agent_admission_challenges(agent_id, created_at desc);
create index if not exists agent_admission_challenges_expires_idx on agent_admission_challenges(expires_at);

-- Platform signing keys (Ed25519); private key stored encrypted at rest (see app config)
create table if not exists platform_signing_keys (
  key_id text primary key,
  alg text not null default 'Ed25519',
  public_key text not null,
  private_key_enc bytea not null,
  created_at timestamptz not null default now(),
  revoked_at timestamptz
);
create index if not exists platform_signing_keys_revoked_idx on platform_signing_keys(revoked_at);
create index if not exists platform_signing_keys_created_idx on platform_signing_keys(created_at);

-- OSS STS credential issuance audit
create table if not exists oss_credential_issuances (
  id uuid primary key default gen_random_uuid(),
  agent_id uuid not null references agents(id) on delete cascade,
  kind text not null,
  scope jsonb not null default '{}'::jsonb,
  expires_at timestamptz not null,
  created_at timestamptz not null default now()
);
create index if not exists oss_credential_issuances_agent_idx on oss_credential_issuances(agent_id, created_at desc);

-- Optional OSS event ingestion (platform-side feed)
create table if not exists oss_events (
  id bigserial primary key,
  object_key text not null,
  event_type text not null,
  occurred_at timestamptz not null,
  payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);
create index if not exists oss_events_occurred_idx on oss_events(occurred_at desc);
create index if not exists oss_events_object_key_idx on oss_events(object_key);

create table if not exists oss_event_acks (
  agent_id uuid primary key references agents(id) on delete cascade,
  last_event_id bigint not null default 0,
  updated_at timestamptz not null default now()
);

-- Persona templates (built-in + owner-submitted for review)
create table if not exists persona_templates (
  id text primary key,
  source text not null default 'built_in' check (source in ('built_in','custom')),
  owner_id uuid references users(id) on delete set null,
  persona jsonb not null,
  review_status text not null default 'pending' check (review_status in ('pending','approved','rejected')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists persona_templates_review_status_idx on persona_templates(review_status);
create index if not exists persona_templates_owner_id_idx on persona_templates(owner_id);

-- Minimal built-in persona templates (can be extended by admin later).
insert into persona_templates (id, source, owner_id, persona, review_status)
values
  (
    'persona_sci_doc_narrator_v1',
    'built_in',
    null,
    '{
      "template_id":"persona_sci_doc_narrator_v1",
      "inspiration":{"kind":"other","reference":"科幻纪录片旁白（风格参考）","note":"仅风格参考，禁止冒充具体身份"},
      "voice":{"tone_tags":["理性","温和","短段落"],"catchphrases":["我们来把系统拆开看看。"]},
      "no_impersonation":true
    }'::jsonb,
    'approved'
  ),
  (
    'persona_xiaotianquan_v1',
    'built_in',
    null,
    '{
      "template_id":"persona_xiaotianquan_v1",
      "inspiration":{"kind":"fictional_character","reference":"西游记·哮天犬（风格参考）","note":"仅风格参考，禁止冒充具体身份"},
      "voice":{"tone_tags":["俏皮","短句","偶尔汪汪"],"catchphrases":["汪！"]},
      "no_impersonation":true
    }'::jsonb,
    'approved'
  )
on conflict (id) do nothing;
