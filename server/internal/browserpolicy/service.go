package browserpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxDomains = 100

var ErrInvalidPolicy = errors.New("invalid browser capture policy")

type Policy struct {
	Enabled        bool      `json:"enabled"`
	AllowedDomains []string  `json:"allowed_domains"`
	Revision       int64     `json:"revision"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type Store interface {
	Get(context.Context, string) (Policy, error)
	Update(context.Context, string, string, bool, []string) (Policy, error)
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func ValidatePolicy(enabled bool, domains []string) error {
	if enabled && len(domains) == 0 {
		return fmt.Errorf("%w: 启用浏览器采集时至少需要一个允许域名", ErrInvalidPolicy)
	}
	if len(domains) > MaxDomains {
		return fmt.Errorf("%w: 允许域名不能超过 %d 个", ErrInvalidPolicy, MaxDomains)
	}
	return nil
}

func NormalizeDomains(values []string) ([]string, error) {
	if len(values) > MaxDomains {
		return nil, fmt.Errorf("%w: 允许域名不能超过 %d 个", ErrInvalidPolicy, MaxDomains)
	}
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf("%w: 域名不能为空", ErrInvalidPolicy)
		}
		candidate := value
		if !strings.Contains(candidate, "://") {
			candidate = "//" + candidate
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.User != nil || (parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("%w: 无效域名 %q", ErrInvalidPolicy, raw)
		}
		host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		if !validHost(host) {
			return nil, fmt.Errorf("%w: 无效域名 %q", ErrInvalidPolicy, raw)
		}
		seen[host] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for domain := range seen {
		result = append(result, domain)
	}
	sort.Strings(result)
	return result, nil
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "*_ \\/") {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func (repository *Repository) Get(ctx context.Context, tenantID string) (Policy, error) {
	var policy Policy
	err := repository.pool.QueryRow(ctx, `SELECT enabled,allowed_domains,revision,updated_at FROM browser_capture_policies WHERE tenant_id=$1`, tenantID).
		Scan(&policy.Enabled, &policy.AllowedDomains, &policy.Revision, &policy.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{AllowedDomains: []string{}}, nil
	}
	return policy, err
}

func (repository *Repository) Update(ctx context.Context, tenantID, actorID string, enabled bool, rawDomains []string) (Policy, error) {
	domains, err := NormalizeDomains(rawDomains)
	if err != nil {
		return Policy{}, err
	}
	if err := ValidatePolicy(enabled, domains); err != nil {
		return Policy{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)
	var policy Policy
	err = tx.QueryRow(ctx, `INSERT INTO browser_capture_policies(tenant_id,enabled,allowed_domains,revision,updated_by)
		VALUES($1,$2,$3,1,$4) ON CONFLICT(tenant_id) DO UPDATE SET enabled=EXCLUDED.enabled,
		allowed_domains=EXCLUDED.allowed_domains,revision=browser_capture_policies.revision+1,updated_by=EXCLUDED.updated_by,updated_at=NOW()
		RETURNING enabled,allowed_domains,revision,updated_at`, tenantID, enabled, domains, actorID).
		Scan(&policy.Enabled, &policy.AllowedDomains, &policy.Revision, &policy.UpdatedAt)
	if err != nil {
		return Policy{}, err
	}
	scope, _ := json.Marshal(map[string]any{"enabled": enabled, "domain_count": len(domains), "revision": policy.Revision})
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(tenant_id,actor_type,actor_id,action,resource_type,resource_id,scope,outcome)
		VALUES($1,'user',$2,'browser_policy.update','browser_capture_policy',$1,$3::jsonb,'success')`, tenantID, actorID, string(scope)); err != nil {
		return Policy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return policy, nil
}
