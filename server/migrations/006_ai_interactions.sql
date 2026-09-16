CREATE INDEX IF NOT EXISTS events_ai_dimensions_idx
    ON events(tenant_id, device_id, project_id, (LOWER(COALESCE(payload->>'role', ''))), occurred_at DESC)
    WHERE event_type = 'ai.message';
