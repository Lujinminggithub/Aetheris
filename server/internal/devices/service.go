package devices

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/auth"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BootstrapRequest struct {
	EnrollmentSecret string `json:"enrollment_secret"`
	TenantID         string `json:"tenant_id"`
	DeviceID         string `json:"device_id"`
	SubjectID        string `json:"subject_id"`
	SubjectName      string `json:"subject_name"`
	ClientVersion    string `json:"client_version"`
	Hostname         string `json:"hostname"`
}
type Credential struct {
	Token     string `json:"device_token"`
	DeviceID  string `json:"device_id"`
	SubjectID string `json:"subject_id"`
	TenantID  string `json:"tenant_id"`
}
type HeartbeatStatus struct {
	Status           string    `json:"status"`
	TenantID         string    `json:"tenant_id"`
	SubjectID        string    `json:"subject_id"`
	DeviceID         string    `json:"device_id"`
	WorkRole         any       `json:"work_role"`
	ServerTime       time.Time `json:"server_time"`
	ServerEventCount int64     `json:"server_event_count"`
	DataGeneration   string    `json:"data_generation"`
}
type Service struct {
	pool             *pgxpool.Pool
	enrollmentSecret string
	defaultTenantID  string
}

func NewService(pool *pgxpool.Pool, enrollmentSecret, defaultTenantID string) *Service {
	return &Service{pool: pool, enrollmentSecret: enrollmentSecret, defaultTenantID: defaultTenantID}
}

func (service *Service) Bootstrap(ctx context.Context, request BootstrapRequest) (Credential, error) {
	if service.enrollmentSecret == "" || request.EnrollmentSecret == "" || request.EnrollmentSecret != service.enrollmentSecret {
		return Credential{}, fmt.Errorf("invalid enrollment secret")
	}
	if request.TenantID != "" && request.TenantID != service.defaultTenantID {
		return Credential{}, fmt.Errorf("tenant is controlled by enrollment configuration")
	}
	if service.defaultTenantID == "" || request.DeviceID == "" || request.SubjectID == "" || request.ClientVersion == "" {
		return Credential{}, fmt.Errorf("device bootstrap fields are required")
	}
	tenantID := service.defaultTenantID
	displayName := strings.TrimSpace(request.SubjectName)
	if displayName == "" {
		displayName = request.SubjectID
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Credential{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,$1) ON CONFLICT(id) DO NOTHING`, tenantID); err != nil {
		return Credential{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO subjects(id,tenant_id,display_name) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING`, request.SubjectID, tenantID, displayName[:min(255, len(displayName))]); err != nil {
		return Credential{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO devices(id,tenant_id,subject_id,client_version,hostname) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO UPDATE SET subject_id=EXCLUDED.subject_id,client_version=EXCLUDED.client_version,hostname=EXCLUDED.hostname,last_seen_at=NOW(),status='online'`, request.DeviceID, tenantID, request.SubjectID, request.ClientVersion, request.Hostname[:min(255, len(request.Hostname))]); err != nil {
		return Credential{}, err
	}
	token, err := randomToken(32)
	if err != nil {
		return Credential{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE device_credentials SET revoked_at=NOW() WHERE tenant_id=$1 AND device_id=$2 AND revoked_at IS NULL`, tenantID, request.DeviceID); err != nil {
		return Credential{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO device_credentials(tenant_id,device_id,token_hash) VALUES($1,$2,$3)`, tenantID, request.DeviceID, digest(token)); err != nil {
		return Credential{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Credential{}, err
	}
	return Credential{Token: token, DeviceID: request.DeviceID, SubjectID: request.SubjectID, TenantID: tenantID}, nil
}

func (service *Service) ResolveCredential(ctx context.Context, token string) (authorization.Principal, error) {
	var tenantID, deviceID, subjectID string
	err := service.pool.QueryRow(ctx, credentialLookupSQL(), digest(token)).Scan(&tenantID, &deviceID, &subjectID)
	if err != nil {
		return authorization.Principal{}, fmt.Errorf("device credential not found")
	}
	if _, err := service.pool.Exec(ctx, `UPDATE device_credentials SET last_used_at=NOW() WHERE token_hash=$1`, digest(token)); err != nil {
		return authorization.Principal{}, err
	}
	return authorization.Principal{Kind: "device", TenantID: tenantID, DeviceID: deviceID, SubjectID: subjectID, Permissions: map[string]bool{"events:ingest": true, "devices:register": true, "projects:register": true, "health:report": true}}, nil
}

func credentialLookupSQL() string {
	return `SELECT c.tenant_id,c.device_id,d.subject_id FROM device_credentials c JOIN devices d ON d.tenant_id=c.tenant_id AND d.id=c.device_id WHERE c.token_hash=$1 AND c.revoked_at IS NULL`
}

func (service *Service) Heartbeat(ctx context.Context, principal authorization.Principal) (HeartbeatStatus, error) {
	now := time.Now().UTC()
	result, err := service.pool.Exec(ctx, `UPDATE devices SET status='online',last_seen_at=$1 WHERE tenant_id=$2 AND id=$3 AND subject_id=$4`, now, principal.TenantID, principal.DeviceID, principal.SubjectID)
	if err != nil {
		return HeartbeatStatus{}, err
	}
	if result.RowsAffected() != 1 {
		return HeartbeatStatus{}, fmt.Errorf("device identity not found")
	}
	var eventCount int64
	var latestIngestedAt time.Time
	if err := service.pool.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MAX(ingested_at),'epoch'::timestamptz) FROM events WHERE tenant_id=$1`, principal.TenantID).Scan(&eventCount, &latestIngestedAt); err != nil {
		return HeartbeatStatus{}, err
	}
	generation := fmt.Sprintf("%d:%d", eventCount, latestIngestedAt.UnixNano())
	return HeartbeatStatus{Status: "online", TenantID: principal.TenantID, SubjectID: principal.SubjectID, DeviceID: principal.DeviceID, ServerTime: now, ServerEventCount: eventCount, DataGeneration: generation}, nil
}

func (service *Service) RevokeCredential(ctx context.Context, principal authorization.Principal) error {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE device_credentials SET revoked_at=NOW() WHERE tenant_id=$1 AND device_id=$2 AND revoked_at IS NULL`, principal.TenantID, principal.DeviceID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE devices SET status='revoked',last_seen_at=NOW() WHERE tenant_id=$1 AND id=$2 AND subject_id=$3`, principal.TenantID, principal.DeviceID, principal.SubjectID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func randomToken(size int) (string, error) { return authToken(size) }
func authToken(size int) (string, error)   { return auth.RandomToken(size) }
func digest(value string) string           { return auth.Digest(value) }
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
