package applicationpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Policy struct {
	Enabled   bool      `json:"enabled"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type Store interface {
	Get(context.Context, string) (Policy, error)
	Update(context.Context, string, string, bool) (Policy, error)
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func DefaultPolicy() Policy                        { return Policy{Enabled: true, Revision: 0} }

func (repository *Repository) Get(ctx context.Context, tenantID string) (Policy, error) {
	var policy Policy
	err := repository.pool.QueryRow(ctx, `SELECT enabled,revision,updated_at FROM application_capture_policies WHERE tenant_id=$1`, tenantID).Scan(&policy.Enabled, &policy.Revision, &policy.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultPolicy(), nil
	}
	return policy, err
}

func (repository *Repository) Update(ctx context.Context, tenantID, actorID string, enabled bool) (Policy, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	var policy Policy
	err = tx.QueryRow(ctx, `INSERT INTO application_capture_policies(tenant_id,enabled,revision,updated_by) VALUES($1,$2,1,$3)
		ON CONFLICT(tenant_id) DO UPDATE SET enabled=EXCLUDED.enabled,revision=application_capture_policies.revision+1,updated_by=EXCLUDED.updated_by,updated_at=NOW()
		RETURNING enabled,revision,updated_at`, tenantID, enabled, actorID).Scan(&policy.Enabled, &policy.Revision, &policy.UpdatedAt)
	if err != nil {
		return Policy{}, err
	}
	scope, _ := json.Marshal(map[string]any{"enabled": enabled, "revision": policy.Revision})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(tenant_id,actor_type,actor_id,action,resource_type,resource_id,scope,outcome)
		VALUES($1,'user',$2,'application_policy.update','application_capture_policy',$1,$3::jsonb,'success')`, tenantID, actorID, string(scope)); err != nil {
		return Policy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return policy, nil
}
