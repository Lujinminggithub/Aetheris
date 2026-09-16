package workroles

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Assignment struct {
	ID, RoleID, Code, Source string
	Version, Priority        int
	ValidFrom                time.Time
	ValidTo                  *time.Time
}

type RoleSnapshot struct {
	RoleID, Code, Source string
	Version              int
}

func ResolveAssignments(assignments []Assignment, at time.Time) (RoleSnapshot, bool) {
	valid := make([]Assignment, 0, len(assignments))
	for _, assignment := range assignments {
		if at.Before(assignment.ValidFrom) || (assignment.ValidTo != nil && !at.Before(*assignment.ValidTo)) {
			continue
		}
		valid = append(valid, assignment)
	}
	if len(valid) == 0 {
		return RoleSnapshot{}, false
	}
	sort.SliceStable(valid, func(i, j int) bool { return valid[i].Priority > valid[j].Priority })
	selected := valid[0]
	return RoleSnapshot{RoleID: selected.RoleID, Code: selected.Code, Version: selected.Version, Source: selected.Source}, true
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (service *Service) Resolve(ctx context.Context, tenantID, subjectID, deviceID, projectID string, at time.Time) (RoleSnapshot, error) {
	rows, err := service.pool.Query(ctx, `SELECT a.id, a.work_role_id, r.code, r.version, a.source, a.valid_from, a.valid_to,
        CASE WHEN a.project_id=$4 THEN 3 WHEN a.subject_id=$2 THEN 2 ELSE 1 END
        FROM work_role_assignments a JOIN work_roles r ON r.tenant_id=a.tenant_id AND r.id=a.work_role_id
        WHERE a.tenant_id=$1 AND (a.subject_id=$2 OR a.device_id=$3 OR (a.subject_id IS NULL AND a.device_id IS NULL))
          AND (a.project_id=$4 OR a.project_id IS NULL) AND a.valid_from <= $5 AND (a.valid_to IS NULL OR a.valid_to > $5) AND r.active`, tenantID, subjectID, deviceID, projectID, at)
	if err != nil {
		return RoleSnapshot{}, err
	}
	defer rows.Close()
	assignments := []Assignment{}
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.RoleID, &a.Code, &a.Version, &a.Source, &a.ValidFrom, &a.ValidTo, &a.Priority); err != nil {
			return RoleSnapshot{}, err
		}
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return RoleSnapshot{}, err
	}
	role, ok := ResolveAssignments(assignments, at)
	if !ok {
		return RoleSnapshot{}, fmt.Errorf("no active work role")
	}
	return role, nil
}
