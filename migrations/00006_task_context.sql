-- Migration: Task Context Enhancement
-- Adds context fields to work_items for enhanced task delivery

-- Add new columns to work_items table
alter table work_items add column if not exists context jsonb default '{}';
alter table work_items add column if not exists available_skills jsonb default '[]';
alter table work_items add column if not exists review_context jsonb default '{}';
alter table work_items add column if not exists scheduled_at timestamptz;

-- Update status check to include 'scheduled'
alter table work_items drop constraint if exists work_items_status_check;
alter table work_items add constraint work_items_status_check check (status in ('offered', 'claimed', 'completed', 'failed', 'scheduled'));

-- Add index for scheduled_at to efficiently find work items ready to be offered
create index if not exists work_items_scheduled_at_idx on work_items(scheduled_at) where status = 'scheduled';
