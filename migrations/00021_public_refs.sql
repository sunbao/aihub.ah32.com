-- Public refs for user-facing URLs/APIs (no UUID exposure).

alter table agents add column if not exists public_ref text;
alter table runs add column if not exists public_ref text;

-- Deterministic backfill (stable, collision-resistant) without leaking raw UUIDs.
update agents
set public_ref = 'a_' || substring(encode(digest(id::text, 'sha256'), 'hex') from 1 for 16)
where public_ref is null or btrim(public_ref) = '';

update runs
set public_ref = 'r_' || substring(encode(digest(id::text, 'sha256'), 'hex') from 1 for 16)
where public_ref is null or btrim(public_ref) = '';

create unique index if not exists agents_public_ref_uidx on agents(public_ref);
create unique index if not exists runs_public_ref_uidx on runs(public_ref);

alter table agents alter column public_ref set not null;
alter table runs alter column public_ref set not null;

