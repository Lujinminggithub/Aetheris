package adminops

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/aetheris-dev/aetheris/server/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ValidateNewPassword(password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return fmt.Errorf("管理员密码至少需要 12 个字符")
	}
	return nil
}

func ResetPassword(ctx context.Context, pool *pgxpool.Pool, username, password string) error {
	if username == "" {
		return fmt.Errorf("管理员用户名不能为空")
	}
	if err := ValidateNewPassword(password); err != nil {
		return err
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID string
	if err := tx.QueryRow(ctx, `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE username=$2 AND active RETURNING id`, passwordHash, username).Scan(&userID); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("管理员用户不存在")
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE api_sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(tenant_id,actor_type,actor_id,action,resource_type,resource_id,scope,outcome)
        SELECT tenant_id,'system','aetheris-admin','admin.password.reset','user',$1,'{}'::jsonb,'success'
        FROM memberships WHERE user_id=$1 GROUP BY tenant_id`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
