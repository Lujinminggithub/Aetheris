package cleaning

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) ListEvidence(ctx context.Context, tenantID string, from, toExclusive time.Time) ([]RawEvidence, error) {
	rows, err := repository.pool.Query(ctx, `SELECT e.event_id,e.tenant_id,e.subject_id,e.device_id,e.project_id,e.event_type,e.source,e.occurred_at,e.ingested_at,e.payload FROM events e WHERE e.tenant_id=$1 AND (e.event_type IN ('terminal.command','ai.message','ai.tool_call','ide.activity','application.activity','browser.page_view','git.commit','git.diff','svn.activity','process.observed') OR e.source LIKE 'core.visual_studio.%') AND e.occurred_at >= $2 AND e.occurred_at < $3 AND NOT EXISTS (SELECT 1 FROM events replacement WHERE replacement.tenant_id=e.tenant_id AND replacement.supersedes_event_id=e.event_id) ORDER BY e.device_id,e.project_id,e.ingested_at,e.event_id`, tenantID, from, toExclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RawEvidence{}
	for rows.Next() {
		var item RawEvidence
		var payload []byte
		if err := rows.Scan(&item.EventID, &item.TenantID, &item.SubjectID, &item.DeviceID, &item.ProjectID, &item.EventType, &item.Source, &item.OccurredAt, &item.IngestedAt, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *Repository) ReplaceRange(ctx context.Context, tenantID string, version int, from, toExclusive time.Time, facts []Fact) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = writeReplacement(ctx, tx, tenantID, version, from, toExclusive, facts); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type rangeFactTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func writeReplacement(ctx context.Context, tx rangeFactTx, tenantID string, version int, from, toExclusive time.Time, facts []Fact) error {
	ids := make([]string, 0, len(facts))
	for _, fact := range facts {
		ids = append(ids, fact.FactID)
		_, err := tx.Exec(ctx, `INSERT INTO clean_event_facts(tenant_id,fact_id,rule_version,subject_id,device_id,project_id,occurred_at,fact_type,event_type,source,activity_type,actor_origin,message_role,ai_tool,command_type,command_summary,command_hash,command_text,quality_state,confidence,merge_method,reason_codes,source_event_ids,canonical_event_id,excluded_from_effectiveness) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
			ON CONFLICT(tenant_id,fact_id,rule_version) DO UPDATE SET subject_id=EXCLUDED.subject_id,device_id=EXCLUDED.device_id,project_id=EXCLUDED.project_id,occurred_at=EXCLUDED.occurred_at,fact_type=EXCLUDED.fact_type,event_type=EXCLUDED.event_type,source=EXCLUDED.source,activity_type=EXCLUDED.activity_type,actor_origin=EXCLUDED.actor_origin,message_role=EXCLUDED.message_role,ai_tool=EXCLUDED.ai_tool,command_type=EXCLUDED.command_type,command_summary=EXCLUDED.command_summary,command_hash=EXCLUDED.command_hash,command_text=EXCLUDED.command_text,quality_state=EXCLUDED.quality_state,confidence=EXCLUDED.confidence,merge_method=EXCLUDED.merge_method,reason_codes=EXCLUDED.reason_codes,source_event_ids=EXCLUDED.source_event_ids,canonical_event_id=EXCLUDED.canonical_event_id,excluded_from_effectiveness=EXCLUDED.excluded_from_effectiveness,updated_at=NOW()`, fact.TenantID, fact.FactID, fact.RuleVersion, fact.SubjectID, fact.DeviceID, fact.ProjectID, fact.OccurredAt, fact.FactType, fact.EventType, fact.Source, fact.ActivityType, fact.ActorOrigin, fact.MessageRole, fact.AITool, fact.CommandType, fact.CommandSummary, fact.CommandHash, fact.CommandText, fact.QualityState, fact.Confidence, fact.MergeMethod, fact.ReasonCodes, fact.SourceEventIDs, fact.CanonicalEventID, fact.ExcludedFromEffectiveness)
		if err != nil {
			return err
		}
	}
	if len(ids) == 0 {
		_, err := tx.Exec(ctx, `DELETE FROM clean_event_facts WHERE tenant_id=$1 AND rule_version=$2 AND occurred_at >= $3 AND occurred_at < $4`, tenantID, version, from, toExclusive)
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM clean_event_facts WHERE tenant_id=$1 AND rule_version=$2 AND occurred_at >= $3 AND occurred_at < $4 AND NOT(fact_id=ANY($5))`, tenantID, version, from, toExclusive, ids)
	return err
}

func (repository *Repository) CreateJob(ctx context.Context, id, tenantID string, from, to time.Time, version int) error {
	_, err := repository.pool.Exec(ctx, `INSERT INTO cleaning_jobs(id,tenant_id,date_from,date_to,rule_version,status) VALUES($1,$2,$3,$4,$5,'queued')`, id, tenantID, from, to, version)
	return err
}

func (repository *Repository) UpdateJob(ctx context.Context, id, status, errorCode string, processed int, facts []Fact) {
	merged, quarantined := Counts(facts)
	_, _ = repository.pool.Exec(ctx, `UPDATE cleaning_jobs SET status=$2,error_code=NULLIF($3,''),processed_events=$4,fact_count=$5,merged_count=$6,quarantined_count=$7,started_at=COALESCE(started_at,NOW()),completed_at=CASE WHEN $2 IN ('completed','failed') THEN NOW() ELSE completed_at END WHERE id=$1`, id, status, errorCode, processed, len(facts), merged, quarantined)
}

func (repository *Repository) Summary(ctx context.Context, tenantID string, from, toExclusive time.Time, version int) (Summary, error) {
	result := Summary{RuleVersion: version}
	err := repository.pool.QueryRow(ctx, `SELECT
        (SELECT COUNT(*) FROM events WHERE tenant_id=$1 AND (event_type IN ('terminal.command','ai.message','ai.tool_call','ide.activity','application.activity','browser.page_view','git.commit','git.diff','svn.activity','process.observed') OR source LIKE 'core.visual_studio.%') AND occurred_at >= $2 AND occurred_at < $3),
        COUNT(*),COUNT(*) FILTER(WHERE quality_state='merged'),COUNT(*) FILTER(WHERE fact_type='command_fragment'),
        COUNT(*) FILTER(WHERE quality_state='quarantined'),COUNT(*) FILTER(WHERE excluded_from_effectiveness)
        FROM clean_event_facts WHERE tenant_id=$1 AND rule_version=$4 AND occurred_at >= $2 AND occurred_at < $3`, tenantID, from, toExclusive, version).Scan(&result.RawEvents, &result.CleanFacts, &result.MergedFacts, &result.CommandFragments, &result.QuarantinedFacts, &result.ExcludedFacts)
	return result, err
}

func (repository *Repository) ListFacts(ctx context.Context, tenantID string, from, toExclusive time.Time, version, limit, offset int, qualityState string) (FactPage, error) {
	page := FactPage{Facts: []FactView{}, Limit: limit, Offset: offset}
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM clean_event_facts WHERE tenant_id=$1 AND rule_version=$2 AND occurred_at >= $3 AND occurred_at < $4 AND ($5='' OR quality_state=$5)`, tenantID, version, from, toExclusive, qualityState).Scan(&page.Total); err != nil {
		return FactPage{}, err
	}
	rows, err := repository.pool.Query(ctx, `SELECT f.fact_id,f.fact_type,f.event_type,f.source,f.activity_type,f.actor_origin,f.project_id,COALESCE(NULLIF(e.payload->>'project_label',''),NULLIF(e.payload->>'project',''),NULLIF(e.payload->>'project_path',''),NULLIF(e.payload->>'cwd',''),NULLIF(e.provenance->>'project_root',''),f.project_id),f.device_id,d.hostname,f.occurred_at,f.quality_state,f.confidence,f.merge_method,f.reason_codes,f.source_event_ids,f.command_text,f.command_summary,f.excluded_from_effectiveness
        FROM clean_event_facts f JOIN devices d ON d.tenant_id=f.tenant_id AND d.id=f.device_id LEFT JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
        WHERE f.tenant_id=$1 AND f.rule_version=$2 AND f.occurred_at >= $3 AND f.occurred_at < $4 AND ($5='' OR f.quality_state=$5)
        ORDER BY f.occurred_at DESC,f.fact_id LIMIT $6 OFFSET $7`, tenantID, version, from, toExclusive, qualityState, limit, offset)
	if err != nil {
		return FactPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item FactView
		var pathHint, commandText, commandSummary string
		if err := rows.Scan(&item.FactID, &item.FactType, &item.EventType, &item.Source, &item.ActivityType, &item.ActorOrigin, &item.ProjectID, &pathHint, &item.DeviceID, &item.DeviceName, &item.OccurredAt, &item.QualityState, &item.Confidence, &item.MergeMethod, &item.ReasonCodes, &item.SourceEventIDs, &commandText, &commandSummary, &item.ExcludedFromEffectiveness); err != nil {
			return FactPage{}, err
		}
		item.ProjectName = safeProjectLabel(pathHint, item.ProjectID)
		item.CommandDisplay = SafeCommandDisplay(Fact{ActorOrigin: item.ActorOrigin, CommandText: commandText, CommandSummary: commandSummary})
		page.Facts = append(page.Facts, item)
	}
	return page, rows.Err()
}

func safeProjectLabel(pathHint, fallback string) string {
	value := strings.TrimRight(strings.TrimSpace(pathHint), `\/`)
	if index := strings.LastIndexAny(value, `\/`); index >= 0 {
		value = value[index+1:]
	}
	if value == "" || (len(value) == 2 && value[1] == ':') {
		return fallback
	}
	return value
}
