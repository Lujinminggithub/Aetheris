ALTER TABLE work_episodes ADD COLUMN IF NOT EXISTS device_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS work_episodes_device_time_idx ON work_episodes(tenant_id, device_id, started_at DESC);
