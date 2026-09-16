CREATE INDEX IF NOT EXISTS events_supersedes_idx
    ON events(tenant_id, supersedes_event_id)
    WHERE supersedes_event_id IS NOT NULL;
