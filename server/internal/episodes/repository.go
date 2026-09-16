package episodes

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) List(ctx context.Context, tenantID string, filter Filter) ([]Episode, error) {
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	rows, err := repository.pool.Query(ctx, `SELECT w.episode_id,w.subject_id,w.device_id,w.project_id,COALESCE(p.display_name,''),w.session_id,w.started_at,w.ended_at,w.title,w.objective,w.outcome,w.confidence,w.needs_review
		FROM work_episodes w LEFT JOIN logical_projects p ON p.tenant_id=w.tenant_id AND p.id=w.logical_project_id WHERE w.tenant_id=$1 AND w.status='active' AND ($2='' OR w.logical_project_id=$2) AND ($3='' OR w.subject_id=$3) AND ($4='' OR w.status=$4) AND ($5='' OR w.device_id=$5)
		ORDER BY started_at DESC,episode_id LIMIT $6 OFFSET $7`, tenantID, filter.ProjectID, filter.SubjectID, filter.Status, filter.DeviceID, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Episode{}
	for rows.Next() {
		var item Episode
		if err := rows.Scan(&item.EpisodeID, &item.SubjectID, &item.DeviceID, &item.ProjectID, &item.ProjectName, &item.SessionID, &item.StartedAt, &item.EndedAt, &item.Title, &item.Objective, &item.Outcome, &item.Confidence, &item.NeedsReview); err != nil {
			return nil, err
		}
		item.Actions = []Action{}
		item.Validations = []Validation{}
		item.Evidence = []Evidence{}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *Repository) Get(ctx context.Context, tenantID, episodeID string) (Episode, error) {
	var item Episode
	if err := repository.pool.QueryRow(ctx, `SELECT w.episode_id,w.subject_id,w.device_id,w.project_id,COALESCE(p.display_name,''),w.session_id,w.started_at,w.ended_at,w.title,w.objective,w.outcome,w.confidence,w.needs_review
		FROM work_episodes w LEFT JOIN logical_projects p ON p.tenant_id=w.tenant_id AND p.id=w.logical_project_id WHERE w.tenant_id=$1 AND w.episode_id=$2 AND w.status='active' ORDER BY w.revision DESC LIMIT 1`, tenantID, episodeID).
		Scan(&item.EpisodeID, &item.SubjectID, &item.DeviceID, &item.ProjectID, &item.ProjectName, &item.SessionID, &item.StartedAt, &item.EndedAt, &item.Title, &item.Objective, &item.Outcome, &item.Confidence, &item.NeedsReview); err != nil {
		return Episode{}, err
	}
	item.Actions = []Action{}
	item.Validations = []Validation{}
	item.Evidence = []Evidence{}
	if err := repository.loadChildren(ctx, tenantID, &item); err != nil {
		return Episode{}, err
	}
	return item, nil
}

func (repository *Repository) loadChildren(ctx context.Context, tenantID string, item *Episode) error {
	var revision int
	if err := repository.pool.QueryRow(ctx, `SELECT revision FROM work_episodes WHERE tenant_id=$1 AND episode_id=$2 AND status='active' ORDER BY revision DESC LIMIT 1`, tenantID, item.EpisodeID).Scan(&revision); err != nil {
		return err
	}
	rows, err := repository.pool.Query(ctx, `SELECT event_id,actor,action_type,summary,occurred_at FROM work_episode_actions WHERE tenant_id=$1 AND episode_id=$2 AND revision=$3 ORDER BY item_index`, tenantID, item.EpisodeID, revision)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value Action
		if err := rows.Scan(&value.EventID, &value.Actor, &value.ActionType, &value.Summary, &value.OccurredAt); err != nil {
			rows.Close()
			return err
		}
		item.Actions = append(item.Actions, value)
	}
	rows.Close()
	rows, err = repository.pool.Query(ctx, `SELECT event_id,validation_type,result,summary,occurred_at FROM work_episode_validations WHERE tenant_id=$1 AND episode_id=$2 AND revision=$3 ORDER BY item_index`, tenantID, item.EpisodeID, revision)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value Validation
		if err := rows.Scan(&value.EventID, &value.ValidationType, &value.Result, &value.Summary, &value.OccurredAt); err != nil {
			rows.Close()
			return err
		}
		item.Validations = append(item.Validations, value)
	}
	rows.Close()
	rows, err = repository.pool.Query(ctx, `SELECT event_id,section,reason FROM work_episode_evidence WHERE tenant_id=$1 AND episode_id=$2 AND revision=$3 ORDER BY section,item_index,event_id`, tenantID, item.EpisodeID, revision)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value Evidence
		if err := rows.Scan(&value.EventID, &value.Section, &value.Reason); err != nil {
			rows.Close()
			return err
		}
		item.Evidence = append(item.Evidence, value)
	}
	rows.Close()
	return nil
}

func (repository *Repository) Save(ctx context.Context, tenantID string, episode Episode, revision int) error {
	if tenantID == "" || episode.EpisodeID == "" || revision < 1 {
		return fmt.Errorf("工作片段身份无效")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE work_episodes SET status='stale' WHERE tenant_id=$1 AND episode_id=$2 AND status='active'`, tenantID, episode.EpisodeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO work_episodes(episode_id,tenant_id,subject_id,device_id,logical_project_id,revision,status,started_at,ended_at,title,objective,context,outcome,confidence,needs_review,generator,generator_version)
		VALUES($1,$2,$3,$4,NULLIF($5,''),$6,'active',$7,$8,$9,$10,'',$11,$12,$13,'deterministic','1')`, episode.EpisodeID, tenantID, episode.SubjectID, episode.DeviceID, episode.ProjectID, revision, episode.StartedAt, episode.EndedAt, episode.Title, episode.Objective, episode.Outcome, episode.Confidence, episode.NeedsReview); err != nil {
		return err
	}
	for index, action := range episode.Actions {
		if _, err := tx.Exec(ctx, `INSERT INTO work_episode_actions(tenant_id,episode_id,revision,item_index,event_id,actor,action_type,summary,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, tenantID, episode.EpisodeID, revision, index, action.EventID, action.Actor, action.ActionType, action.Summary, action.OccurredAt); err != nil {
			return err
		}
	}
	for index, validation := range episode.Validations {
		if _, err := tx.Exec(ctx, `INSERT INTO work_episode_validations(tenant_id,episode_id,revision,item_index,event_id,validation_type,result,summary,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, tenantID, episode.EpisodeID, revision, index, validation.EventID, validation.ValidationType, validation.Result, validation.Summary, validation.OccurredAt); err != nil {
			return err
		}
	}
	for index, evidence := range episode.Evidence {
		if _, err := tx.Exec(ctx, `INSERT INTO work_episode_evidence(tenant_id,episode_id,revision,section,item_index,event_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7)`, tenantID, episode.EpisodeID, revision, evidence.Section, index, evidence.EventID, evidence.Reason); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (repository *Repository) RecomputeRecent(ctx context.Context, tenantID string, since time.Time) (int, error) {
	return repository.RecomputeRecentWithEnrichment(ctx, tenantID, since, nil, "", "")
}

func (repository *Repository) RecomputeRecentWithEnrichment(ctx context.Context, tenantID string, since time.Time, ollamaClient *http.Client, ollamaURL, ollamaModel string) (int, error) {
	rows, err := repository.pool.Query(ctx, `SELECT f.canonical_event_id,f.subject_id,f.device_id,COALESCE(ep.logical_project_id,''),e.session_id,f.event_type,f.message_role,
		COALESCE(e.payload->>'content',''),CASE f.event_type
			WHEN 'ide.file_opened' THEN '打开代码文件'
			WHEN 'ide.file_edited' THEN '编辑代码文件'
			WHEN 'ide.file_saved' THEN '保存代码文件'
			WHEN 'ide.file_closed' THEN '关闭代码文件'
			WHEN 'ide.workspace_changed' THEN '切换工作区'
			WHEN 'ide.extension_changed' THEN '开发扩展环境变化'
			WHEN 'application.activity' THEN CASE WHEN COALESCE(e.payload->>'application_name','')<>'' AND COALESCE(e.payload->>'window_context','')<>'' THEN '在 ' || (e.payload->>'application_name') || ' 中' || (e.payload->>'window_context') ELSE '' END
			ELSE COALESCE(f.command_summary,'') END,f.occurred_at
		FROM clean_event_facts f JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
		LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id
		WHERE f.tenant_id=$1 AND f.occurred_at >= $2 AND f.rule_version=(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1)
		AND f.quality_state IN ('accepted','merged') AND NOT f.excluded_from_effectiveness
		ORDER BY f.occurred_at,f.fact_id`, tenantID, since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	facts := []Fact{}
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.EventID, &fact.SubjectID, &fact.DeviceID, &fact.ProjectID, &fact.SessionID, &fact.EventType, &fact.Role, &fact.Content, &fact.Summary, &fact.OccurredAt); err != nil {
			return 0, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, episode := range Build(facts) {
		if ollamaClient != nil && ollamaURL != "" {
			if summary, summaryErr := SummarizeWithOllama(ctx, ollamaClient, ollamaURL, ollamaModel, episode); summaryErr == nil {
				if summary.Title != "" {
					episode.Title = summary.Title
				}
				if summary.Objective != "" {
					episode.Objective = summary.Objective
				}
				if summary.Outcome != "" {
					episode.Outcome = summary.Outcome
				}
			}
		}
		var revision int
		if err := repository.pool.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM work_episodes WHERE tenant_id=$1 AND episode_id=$2`, tenantID, episode.EpisodeID).Scan(&revision); err != nil {
			return count, err
		}
		if err := repository.Save(ctx, tenantID, episode, revision); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
