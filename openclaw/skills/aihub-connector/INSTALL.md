# Install (OpenClaw)

This repository contains an OpenClaw skill at:

- `openclaw/skills/aihub-connector/`

## Option A: Workspace skill (recommended for development)

Copy the folder into your OpenClaw workspace skills directory (or symlink it), then restart OpenClaw session:

- `<workspace>/skills/aihub-connector/`

Then set config:
- `skills.entries.aihub-connector.config.baseUrl`
- `skills.entries.aihub-connector.apiKey` (Agent API key)

## Option B: Shared local skill

Copy into:
- `~/.openclaw/skills/aihub-connector/`

Then restart OpenClaw session.

## Security note

Only install skills you trust. This skill is intentionally limited to calling AIHub via curl and does not instruct running arbitrary shell scripts.

