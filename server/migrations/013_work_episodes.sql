CREATE TABLE IF NOT EXISTS work_episodes (
    episode_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subject_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    logical_project_id TEXT,
    revision INTEGER NOT NULL CHECK (revision > 0),
    status TEXT NOT NULL CHECK (status IN ('active', 'stale', 'needs_review', 'deleted')),
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    objective TEXT NOT NULL DEFAULT '',
    context TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT '',
    confidence TEXT NOT NULL CHECK (confidence IN ('high', 'medium', 'low')),
    needs_review BOOLEAN NOT NULL DEFAULT TRUE,
    generator TEXT NOT NULL DEFAULT 'deterministic',
    generator_version TEXT NOT NULL DEFAULT '1',
    attribution_rule_version INTEGER,
    cleaning_rule_version INTEGER,
    supersedes_revision INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, episode_id, revision),
    FOREIGN KEY (tenant_id, subject_id) REFERENCES subjects(tenant_id, id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id),
    FOREIGN KEY (tenant_id, logical_project_id) REFERENCES logical_projects(tenant_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS work_episodes_active_idx
    ON work_episodes(tenant_id, episode_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS work_episodes_scope_time_idx
    ON work_episodes(tenant_id, subject_id, logical_project_id, started_at DESC);

CREATE TABLE IF NOT EXISTS work_episode_actions (
    tenant_id TEXT NOT NULL,
    episode_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    item_index INTEGER NOT NULL CHECK (item_index >= 0),
    event_id TEXT NOT NULL,
    actor TEXT NOT NULL,
    action_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, episode_id, revision, item_index),
    FOREIGN KEY (tenant_id, episode_id, revision) REFERENCES work_episodes(tenant_id, episode_id, revision) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_episode_decisions (
    tenant_id TEXT NOT NULL,
    episode_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    item_index INTEGER NOT NULL CHECK (item_index >= 0),
    event_id TEXT NOT NULL,
    decision TEXT NOT NULL,
    rationale TEXT NOT NULL DEFAULT '',
    confidence TEXT NOT NULL CHECK (confidence IN ('high', 'medium', 'low')),
    PRIMARY KEY (tenant_id, episode_id, revision, item_index),
    FOREIGN KEY (tenant_id, episode_id, revision) REFERENCES work_episodes(tenant_id, episode_id, revision) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_episode_validations (
    tenant_id TEXT NOT NULL,
    episode_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    item_index INTEGER NOT NULL CHECK (item_index >= 0),
    event_id TEXT NOT NULL,
    validation_type TEXT NOT NULL,
    result TEXT NOT NULL,
    summary TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, episode_id, revision, item_index),
    FOREIGN KEY (tenant_id, episode_id, revision) REFERENCES work_episodes(tenant_id, episode_id, revision) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_episode_next_actions (
    tenant_id TEXT NOT NULL,
    episode_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    item_index INTEGER NOT NULL CHECK (item_index >= 0),
    event_id TEXT,
    summary TEXT NOT NULL,
    PRIMARY KEY (tenant_id, episode_id, revision, item_index),
    FOREIGN KEY (tenant_id, episode_id, revision) REFERENCES work_episodes(tenant_id, episode_id, revision) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_episode_evidence (
    tenant_id TEXT NOT NULL,
    episode_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    section TEXT NOT NULL,
    item_index INTEGER NOT NULL CHECK (item_index >= 0),
    event_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    PRIMARY KEY (tenant_id, episode_id, revision, section, item_index, event_id),
    FOREIGN KEY (tenant_id, episode_id, revision) REFERENCES work_episodes(tenant_id, episode_id, revision) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, event_id) ON DELETE CASCADE
);

ALTER TABLE work_episodes ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_work_episodes ON work_episodes
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
ALTER TABLE work_episode_actions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_work_episode_actions ON work_episode_actions
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
ALTER TABLE work_episode_decisions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_work_episode_decisions ON work_episode_decisions
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
ALTER TABLE work_episode_validations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_work_episode_validations ON work_episode_validations
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
ALTER TABLE work_episode_next_actions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_work_episode_next_actions ON work_episode_next_actions
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
ALTER TABLE work_episode_evidence ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_work_episode_evidence ON work_episode_evidence
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
