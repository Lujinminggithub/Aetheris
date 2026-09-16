ALTER TABLE adapter_health_snapshots
    ADD COLUMN IF NOT EXISTS component_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS component_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS protocol_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS vscode_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_component_heartbeat_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS pending_events INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sent_events BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS dropped_events BIGINT NOT NULL DEFAULT 0;

ALTER TABLE adapter_health_snapshots
    DROP CONSTRAINT IF EXISTS adapter_health_component_state_check;
ALTER TABLE adapter_health_snapshots
    ADD CONSTRAINT adapter_health_component_state_check CHECK (
        component_state IN ('', 'active', 'install_declined', 'paused_by_user', 'not_installed',
            'pending_install', 'awaiting_activation', 'inactive_in_vscode', 'bridge_offline',
            'unsupported_remote_host', 'incompatible', 'error')
    );

ALTER TABLE adapter_health_snapshots
    DROP CONSTRAINT IF EXISTS adapter_health_component_counts_check;
ALTER TABLE adapter_health_snapshots
    ADD CONSTRAINT adapter_health_component_counts_check CHECK (
        protocol_version >= 0 AND pending_events >= 0 AND sent_events >= 0 AND dropped_events >= 0
    );

CREATE INDEX IF NOT EXISTS adapter_health_component_state_idx
    ON adapter_health_snapshots(tenant_id, component_state, updated_at DESC)
    WHERE component_state <> '';
