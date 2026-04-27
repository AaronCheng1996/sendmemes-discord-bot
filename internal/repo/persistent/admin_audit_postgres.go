package persistent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// AdminAuditRepo persists admin audit events.
type AdminAuditRepo struct {
	*postgres.Postgres
}

// NewAdminAuditRepo creates a new admin audit repository.
func NewAdminAuditRepo(pg *postgres.Postgres) *AdminAuditRepo {
	return &AdminAuditRepo{Postgres: pg}
}

// Insert stores one audit row.
func (r *AdminAuditRepo) Insert(ctx context.Context, log entity.AdminAuditLog) error {
	meta, err := json.Marshal(log.Metadata)
	if err != nil {
		return fmt.Errorf("AdminAuditRepo - Insert - json.Marshal: %w", err)
	}

	sql, args, err := r.Builder.
		Insert("admin_audit_logs").
		Columns("actor", "action", "target_type", "target_id", "metadata").
		Values(log.Actor, log.Action, log.TargetType, log.TargetID, meta).
		ToSql()
	if err != nil {
		return fmt.Errorf("AdminAuditRepo - Insert - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AdminAuditRepo - Insert - Exec: %w", err)
	}
	return nil
}
