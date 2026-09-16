CREATE TABLE IF NOT EXISTS browser_capture_policies (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    allowed_domains TEXT[] NOT NULL DEFAULT '{}',
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (NOT enabled OR cardinality(allowed_domains) > 0),
    CHECK (cardinality(allowed_domains) <= 100)
);

ALTER TABLE browser_capture_policies ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_browser_capture_policies ON browser_capture_policies
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE subject_effectiveness_daily
    ADD COLUMN IF NOT EXISTS browser_events INTEGER NOT NULL DEFAULT 0;

INSERT INTO subject_effectiveness_daily(
    tenant_id,subject_id,local_date,timezone,metric_definition_version,active_window_minutes,
    session_count,total_session_minutes,longest_session_minutes,focus_block_count,focus_block_minutes,
    context_switch_count,delivery_events,coding_events,terminal_events,ai_collaboration_events,browser_events,other_events,
    project_breakdown,work_role_breakdown,source_counts,device_ids,evidence_event_ids,first_event_at,last_event_at,computed_at)
SELECT tenant_id,subject_id,local_date,timezone,4,active_window_minutes,
    session_count,total_session_minutes,longest_session_minutes,focus_block_count,focus_block_minutes,
    context_switch_count,delivery_events,coding_events,terminal_events,ai_collaboration_events,0,other_events,
    project_breakdown,work_role_breakdown,source_counts,device_ids,evidence_event_ids,first_event_at,last_event_at,NOW()
FROM subject_effectiveness_daily
WHERE metric_definition_version=3
ON CONFLICT DO NOTHING;
