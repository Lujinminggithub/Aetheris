package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) Register(ctx context.Context, principal authorization.Principal, batch RegistrationBatch) (BatchResult, error) {
	if principal.Kind != "device" || principal.TenantID == "" || principal.DeviceID == "" {
		return BatchResult{}, fmt.Errorf("设备项目注册身份无效")
	}
	if len(batch.Projects) == 0 || len(batch.Projects) > 100 {
		return BatchResult{}, fmt.Errorf("项目注册批次数量无效")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return BatchResult{}, err
	}
	defer tx.Rollback(ctx)
	result := BatchResult{Projects: make([]RegistrationResult, 0, len(batch.Projects))}
	for _, registration := range batch.Projects {
		if err := ValidateRegistration(registration); err != nil {
			return BatchResult{}, err
		}
		resolved, err := resolveInTransaction(ctx, tx, principal, registration)
		if err != nil {
			return BatchResult{}, err
		}
		result.Projects = append(result.Projects, resolved)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO project_registry_state(tenant_id,revision,updated_at) VALUES($1,1,NOW())
		ON CONFLICT(tenant_id) DO UPDATE SET revision=project_registry_state.revision+1,updated_at=NOW()
		RETURNING revision`, principal.TenantID).Scan(&result.RegistryRevision); err != nil {
		return BatchResult{}, err
	}
	scope, _ := json.Marshal(map[string]any{"device_id": principal.DeviceID, "project_count": len(batch.Projects), "registry_revision": result.RegistryRevision})
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(tenant_id,actor_type,actor_id,action,resource_type,resource_id,scope,outcome)
		VALUES($1,'device',$2,'project.register','project_registry',$1,$3::jsonb,'success')`, principal.TenantID, principal.DeviceID, scope); err != nil {
		return BatchResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BatchResult{}, err
	}
	return result, nil
}

func resolveInTransaction(ctx context.Context, tx pgx.Tx, principal authorization.Principal, registration Registration) (RegistrationResult, error) {
	var exact Location
	err := tx.QueryRow(ctx, `SELECT id,logical_project_id,device_id,local_project_id,display_name,root_fingerprint,workspace_kind,worktree_name,active,key_version,metadata_revision,first_seen_at,last_seen_at
		FROM project_locations WHERE tenant_id=$1 AND device_id=$2 AND local_project_id=$3`, principal.TenantID, principal.DeviceID, registration.LocalProjectID).
		Scan(&exact.ID, &exact.LogicalProjectID, &exact.DeviceID, &exact.LocalProjectID, &exact.DisplayName, &exact.RootFingerprint, &exact.WorkspaceKind, &exact.WorktreeName, &exact.Active, &exact.KeyVersion, &exact.MetadataRevision, &exact.FirstSeenAt, &exact.LastSeenAt)
	if err != nil && err != pgx.ErrNoRows {
		return RegistrationResult{}, err
	}
	var exactPointer *Location
	if err == nil {
		exactPointer = &exact
	}
	candidates, err := matchingRemoteProjects(ctx, tx, principal.TenantID, registration.RemoteFingerprint)
	if err != nil {
		return RegistrationResult{}, err
	}
	resolution := ResolveRegistration(registration, exactPointer, candidates)
	logicalID := resolution.LogicalProjectID
	if resolution.Create || resolution.NeedsReview {
		logicalID, err = createOrResolveLogicalProject(ctx, tx, principal.TenantID, registration, resolution.NeedsReview)
		if err != nil {
			return RegistrationResult{}, err
		}
		if !resolution.NeedsReview && registration.RemoteFingerprint != "" {
			if logicalID != "" && resolution.Method == "new_project" {
				resolution.Method = "new_project"
			}
		}
	}
	if logicalID == "" {
		return RegistrationResult{}, fmt.Errorf("无法确定逻辑项目")
	}
	locationID := exact.ID
	if locationID == "" {
		locationID, err = newIdentifier("project-location-")
		if err != nil {
			return RegistrationResult{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO project_locations(id,tenant_id,logical_project_id,device_id,local_project_id,display_name,root_fingerprint,workspace_kind,worktree_name,active,key_version,metadata_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT(tenant_id,device_id,local_project_id) DO UPDATE SET
			display_name=EXCLUDED.display_name,root_fingerprint=EXCLUDED.root_fingerprint,workspace_kind=EXCLUDED.workspace_kind,
			worktree_name=EXCLUDED.worktree_name,active=EXCLUDED.active,key_version=EXCLUDED.key_version,
			metadata_revision=EXCLUDED.metadata_revision,last_seen_at=NOW()
		WHERE project_locations.metadata_revision <= EXCLUDED.metadata_revision`,
		locationID, principal.TenantID, logicalID, principal.DeviceID, registration.LocalProjectID,
		strings.TrimSpace(registration.DisplayName), registration.RootFingerprint, registration.WorkspaceKind,
		registration.WorktreeName, registration.Active, registration.KeyVersion, registration.MetadataRevision)
	if err != nil {
		return RegistrationResult{}, err
	}
	return RegistrationResult{
		LocalProjectID: registration.LocalProjectID, LogicalProjectID: logicalID, DisplayName: strings.TrimSpace(registration.DisplayName),
		Resolution: resolution.Method, NeedsReview: resolution.NeedsReview,
	}, nil
}

func matchingRemoteProjects(ctx context.Context, tx pgx.Tx, tenantID, fingerprint string) ([]LogicalProject, error) {
	if fingerprint == "" {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT id,display_name,vcs,remote_fingerprint,status,metadata_revision,created_at,updated_at
		FROM logical_projects WHERE tenant_id=$1 AND remote_fingerprint=$2`, tenantID, fingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []LogicalProject{}
	for rows.Next() {
		var item LogicalProject
		item.TenantID = tenantID
		if err := rows.Scan(&item.ID, &item.DisplayName, &item.VCS, &item.RemoteFingerprint, &item.Status, &item.MetadataRevision, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func createOrResolveLogicalProject(ctx context.Context, tx pgx.Tx, tenantID string, registration Registration, needsReview bool) (string, error) {
	id, err := newIdentifier("logical-project-")
	if err != nil {
		return "", err
	}
	status := "active"
	if needsReview {
		status = "needs_review"
	}
	tag, err := tx.Exec(ctx, `INSERT INTO logical_projects(id,tenant_id,display_name,vcs,remote_fingerprint,status,metadata_revision)
		VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7) ON CONFLICT DO NOTHING`,
		id, tenantID, strings.TrimSpace(registration.DisplayName), registration.VCS, registration.RemoteFingerprint, status, registration.MetadataRevision)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 1 {
		return id, nil
	}
	if registration.RemoteFingerprint == "" {
		return "", fmt.Errorf("逻辑项目创建冲突")
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM logical_projects WHERE tenant_id=$1 AND vcs=$2 AND remote_fingerprint=$3`, tenantID, registration.VCS, registration.RemoteFingerprint).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (repository *Repository) List(ctx context.Context, tenantID string) ([]ProjectView, error) {
	rows, err := repository.pool.Query(ctx, `SELECT p.id,p.display_name,p.vcs,p.status,p.metadata_revision,
		l.id,l.logical_project_id,l.device_id,l.local_project_id,l.display_name,l.root_fingerprint,l.workspace_kind,l.worktree_name,l.active,l.key_version,l.metadata_revision,l.first_seen_at,l.last_seen_at
		FROM logical_projects p LEFT JOIN project_locations l ON l.tenant_id=p.tenant_id AND l.logical_project_id=p.id
		WHERE p.tenant_id=$1 ORDER BY p.display_name,p.id,l.device_id,l.local_project_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProjectView{}
	indexes := map[string]int{}
	for rows.Next() {
		var project ProjectView
		var location Location
		var locationID, logicalID, deviceID, localID, locationName, rootFingerprint, workspaceKind, worktreeName *string
		var active *bool
		var keyVersion *int
		var locationRevision *int64
		var firstSeen, lastSeen *time.Time
		if err := rows.Scan(&project.ID, &project.DisplayName, &project.VCS, &project.Status, &project.MetadataRevision,
			&locationID, &logicalID, &deviceID, &localID, &locationName, &rootFingerprint, &workspaceKind, &worktreeName,
			&active, &keyVersion, &locationRevision, &firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		index, found := indexes[project.ID]
		if !found {
			project.Locations = []Location{}
			items = append(items, project)
			index = len(items) - 1
			indexes[project.ID] = index
		}
		if locationID != nil {
			location.ID, location.LogicalProjectID, location.DeviceID, location.LocalProjectID = *locationID, *logicalID, *deviceID, *localID
			location.DisplayName, location.RootFingerprint, location.WorkspaceKind, location.WorktreeName = *locationName, *rootFingerprint, *workspaceKind, *worktreeName
			location.Active, location.KeyVersion, location.MetadataRevision = *active, *keyVersion, *locationRevision
			if firstSeen != nil {
				location.FirstSeenAt = *firstSeen
			}
			if lastSeen != nil {
				location.LastSeenAt = *lastSeen
			}
			items[index].Locations = append(items[index].Locations, location)
			items[index].LocationCount++
		}
	}
	return items, rows.Err()
}

func newIdentifier(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}
