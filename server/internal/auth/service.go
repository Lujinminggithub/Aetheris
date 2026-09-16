package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Session struct {
	Token, CSRF string
	ExpiresAt   time.Time
	Username    string
	AccessRole  string
	Principal   authorization.Principal
}

type Service struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewService(pool *pgxpool.Pool, ttl time.Duration) *Service {
	return &Service{pool: pool, ttl: ttl}
}

func (service *Service) EnsureBootstrapAdmin(ctx context.Context, tenantID, username, password string) error {
	if password == "" {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = service.pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,$1) ON CONFLICT(id) DO NOTHING`, tenantID)
	if err != nil {
		return err
	}
	_, err = service.pool.Exec(ctx, `INSERT INTO users(id,username,password_hash) VALUES($1,$2,$3) ON CONFLICT(username) DO NOTHING`, "user-"+username, username, hash)
	if err != nil {
		return err
	}
	_, err = service.pool.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role_id) SELECT $1,id,'platform_admin' FROM users WHERE username=$2 ON CONFLICT DO NOTHING`, tenantID, username)
	return err
}

func (service *Service) Login(ctx context.Context, username, password string) (Session, error) {
	var userID, passwordHash, tenantID, roleID string
	err := service.pool.QueryRow(ctx, `SELECT u.id,u.password_hash,m.tenant_id,m.role_id FROM users u JOIN memberships m ON m.user_id=u.id WHERE u.username=$1 AND u.active`, username).Scan(&userID, &passwordHash, &tenantID, &roleID)
	if err != nil || !VerifyPassword(passwordHash, password) {
		return Session{}, fmt.Errorf("invalid credentials")
	}
	permissions := map[string]bool{}
	rows, err := service.pool.Query(ctx, `SELECT p.id FROM access_role_permissions rp JOIN permissions p ON p.id=rp.permission_id WHERE rp.role_id=$1`, roleID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var permission string
			if rows.Scan(&permission) == nil {
				permissions[permission] = true
			}
		}
	}
	token, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return Session{}, err
	}
	expires := time.Now().UTC().Add(service.ttl)
	if _, err := service.pool.Exec(ctx, `INSERT INTO api_sessions(user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4)`, userID, digest(token), digest(csrf), expires); err != nil {
		return Session{}, err
	}
	return Session{Token: token, CSRF: csrf, ExpiresAt: expires, Username: username, AccessRole: roleID, Principal: authorization.Principal{Kind: "user", ID: userID, TenantID: tenantID, AccessRole: roleID, Permissions: permissions}}, nil
}

func (service *Service) ResolveSession(ctx context.Context, token string) (authorization.Principal, string, error) {
	var userID, tenantID, roleID string
	err := service.pool.QueryRow(ctx, `SELECT u.id,m.tenant_id,m.role_id FROM api_sessions s JOIN users u ON u.id=s.user_id JOIN memberships m ON m.user_id=u.id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>NOW()`, digest(token)).Scan(&userID, &tenantID, &roleID)
	if err != nil {
		return authorization.Principal{}, "", fmt.Errorf("session not found")
	}
	permissions := map[string]bool{}
	rows, err := service.pool.Query(ctx, `SELECT p.id FROM access_role_permissions rp JOIN permissions p ON p.id=rp.permission_id WHERE rp.role_id=$1`, roleID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var permission string
			if rows.Scan(&permission) == nil {
				permissions[permission] = true
			}
		}
	}
	return authorization.Principal{Kind: "user", ID: userID, TenantID: tenantID, AccessRole: roleID, Permissions: permissions}, roleID, nil
}

func (service *Service) Logout(ctx context.Context, token string) error {
	_, err := service.pool.Exec(ctx, `UPDATE api_sessions SET revoked_at=NOW() WHERE token_hash=$1`, digest(token))
	return err
}

func randomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func RandomToken(size int) (string, error) { return randomToken(size) }
func Digest(value string) string           { return digest(value) }
