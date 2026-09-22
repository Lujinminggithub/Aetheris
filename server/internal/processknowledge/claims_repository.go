package processknowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ClaimStore interface {
	ListClaims(context.Context, ClaimFilter) ([]Claim, int, error)
	ReviewClaim(context.Context, ClaimReviewCommand) (Claim, error)
}

func (repository *Repository) ListClaims(ctx context.Context, filter ClaimFilter) ([]Claim, int, error) {
	where := `tenant_id=$1 AND ($2='' OR domain=$2) AND ($3='' OR lifecycle_state=$3) AND ($4='' OR validation_state=$4)`
	var total int
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM process_knowledge_claims WHERE `+where, filter.TenantID, filter.Domain, filter.LifecycleState, filter.ValidationState).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := repository.pool.Query(ctx, `SELECT claim_id,knowledge_id,revision,sequence,domain,entities,problem,claim,applicability,validation_state,lifecycle_state,evidence_ids,created_at,updated_at FROM process_knowledge_claims WHERE `+where+` ORDER BY updated_at DESC,claim_id LIMIT $5 OFFSET $6`, filter.TenantID, filter.Domain, filter.LifecycleState, filter.ValidationState, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []Claim{}
	for rows.Next() {
		var item Claim
		if err = rows.Scan(&item.ID, &item.KnowledgeID, &item.Revision, &item.Sequence, &item.Domain, &item.Entities, &item.Problem, &item.Claim, &item.Applicability, &item.ValidationState, &item.LifecycleState, &item.EvidenceIDs, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (repository *Repository) ReviewClaim(ctx context.Context, command ClaimReviewCommand) (Claim, error) {
	if command.Action != "confirm" && command.Action != "reject" {
		return Claim{}, errors.New("原子知识审核动作无效")
	}
	if len([]rune(strings.TrimSpace(command.Reason))) < 10 {
		return Claim{}, errors.New("审核理由至少需要 10 个字符")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Claim{}, err
	}
	defer tx.Rollback(ctx)
	state := "confirmed"
	if command.Action == "reject" {
		state = "rejected"
	}
	result := tx.QueryRow(ctx, `UPDATE process_knowledge_claims SET lifecycle_state=$3,updated_at=NOW() WHERE tenant_id=$1 AND claim_id=$2 RETURNING claim_id,knowledge_id,revision,sequence,domain,entities,problem,claim,applicability,validation_state,lifecycle_state,evidence_ids,created_at,updated_at`, command.TenantID, command.ClaimID, state)
	var item Claim
	if err = result.Scan(&item.ID, &item.KnowledgeID, &item.Revision, &item.Sequence, &item.Domain, &item.Entities, &item.Problem, &item.Claim, &item.Applicability, &item.ValidationState, &item.LifecycleState, &item.EvidenceIDs, &item.CreatedAt, &item.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, errors.New("原子知识不存在")
	} else if err != nil {
		return Claim{}, err
	}
	value := make([]byte, 16)
	if _, err = rand.Read(value); err != nil {
		return Claim{}, err
	}
	reviewID := "claim-review-" + hex.EncodeToString(value)
	if _, err = tx.Exec(ctx, `INSERT INTO process_knowledge_claim_reviews(tenant_id,review_id,claim_id,action,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6)`, command.TenantID, reviewID, command.ClaimID, command.Action, command.ActorID, strings.TrimSpace(command.Reason)); err != nil {
		return Claim{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Claim{}, err
	}
	item.Reviews = []ClaimReview{{ID: reviewID, Action: command.Action, ActorID: command.ActorID, Reason: strings.TrimSpace(command.Reason)}}
	return item, nil
}
