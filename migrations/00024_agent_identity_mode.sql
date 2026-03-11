-- Agent identity mode: which "identity system" the agent should follow when executing work items.
-- - 'card': AIHub Agent Card (prompt_view/persona) is enforced and injected into stage_context.
-- - 'openclaw': OpenClaw local workspace identity (SOUL.md / IDENTITY.md / USER.md) is primary; AIHub will not inject card prompts.

alter table agents add column if not exists identity_mode text not null default 'card';

do $$
begin
  alter table agents add constraint agents_identity_mode_chk
    check (identity_mode in ('card','openclaw'));
exception when duplicate_object then null;
end $$;

