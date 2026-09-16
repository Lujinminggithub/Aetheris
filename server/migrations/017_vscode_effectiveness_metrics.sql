ALTER TABLE subject_effectiveness_daily
    ADD COLUMN IF NOT EXISTS ide_file_opened_events INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ide_edit_sessions INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ide_file_saved_events INTEGER NOT NULL DEFAULT 0;

ALTER TABLE clean_event_facts DROP CONSTRAINT IF EXISTS clean_event_facts_activity_type_check;
ALTER TABLE clean_event_facts ADD CONSTRAINT clean_event_facts_activity_type_check
    CHECK (activity_type IN ('ai', 'terminal', 'ide', 'delivery', 'browser', 'version_control', 'other'));
