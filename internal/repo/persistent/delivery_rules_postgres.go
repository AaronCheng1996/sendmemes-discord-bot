package persistent

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// DeliveryRulesRepo stores Discord delivery rules in postgres.
type DeliveryRulesRepo struct {
	*postgres.Postgres
}

// NewDeliveryRulesRepo creates a new delivery rules repository.
func NewDeliveryRulesRepo(pg *postgres.Postgres) *DeliveryRulesRepo {
	return &DeliveryRulesRepo{Postgres: pg}
}

func deliveryRuleSelect(r *DeliveryRulesRepo) sq.SelectBuilder {
	return r.Builder.
		Select("id", "name", "guild_id", "trigger_type", "channel_id",
			"COALESCE(send_interval, '')", "history_size", "enabled", "created_at", "updated_at").
		From("delivery_rules")
}

func scanDeliveryRule(row pgx.Row) (entity.DeliveryRule, error) {
	var rule entity.DeliveryRule
	if err := row.Scan(
		&rule.ID, &rule.Name, &rule.GuildID, &rule.TriggerType, &rule.ChannelID,
		&rule.SendInterval, &rule.HistorySize, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	); err != nil {
		return entity.DeliveryRule{}, err
	}
	return rule, nil
}

func (r *DeliveryRulesRepo) queryRules(ctx context.Context, caller, sql string, args []interface{}) ([]entity.DeliveryRule, error) {
	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s - Query: %w", caller, err)
	}
	defer rows.Close()

	rules := make([]entity.DeliveryRule, 0)
	for rows.Next() {
		rule, scanErr := scanDeliveryRule(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("%s - Scan: %w", caller, scanErr)
		}
		rules = append(rules, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s - rows.Err: %w", caller, err)
	}
	return rules, nil
}

// List returns all rules ordered by id.
func (r *DeliveryRulesRepo) List(ctx context.Context) ([]entity.DeliveryRule, error) {
	sql, args, err := deliveryRuleSelect(r).OrderBy("id ASC").ToSql()
	if err != nil {
		return nil, fmt.Errorf("DeliveryRulesRepo - List - r.Builder: %w", err)
	}
	return r.queryRules(ctx, "DeliveryRulesRepo - List", sql, args)
}

// ListActiveByTrigger returns enabled rules of the given trigger type.
func (r *DeliveryRulesRepo) ListActiveByTrigger(ctx context.Context, triggerType string) ([]entity.DeliveryRule, error) {
	sql, args, err := deliveryRuleSelect(r).
		Where(sq.Eq{"trigger_type": triggerType, "enabled": true}).
		OrderBy("id ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("DeliveryRulesRepo - ListActiveByTrigger - r.Builder: %w", err)
	}
	return r.queryRules(ctx, "DeliveryRulesRepo - ListActiveByTrigger", sql, args)
}

// GetByID returns a rule by primary key.
func (r *DeliveryRulesRepo) GetByID(ctx context.Context, id int64) (entity.DeliveryRule, error) {
	sql, args, err := deliveryRuleSelect(r).Where("id = ?", id).Limit(1).ToSql()
	if err != nil {
		return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - GetByID - r.Builder: %w", err)
	}
	rule, err := scanDeliveryRule(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - GetByID - rule %d not found", id)
		}
		return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - GetByID - QueryRow: %w", err)
	}
	return rule, nil
}

const deliveryRuleReturning = "RETURNING id, name, guild_id, trigger_type, channel_id, COALESCE(send_interval, ''), history_size, enabled, created_at, updated_at"

// Create inserts a new rule.
func (r *DeliveryRulesRepo) Create(ctx context.Context, rule entity.DeliveryRule) (entity.DeliveryRule, error) {
	sql, args, err := r.Builder.
		Insert("delivery_rules").
		Columns("name", "guild_id", "trigger_type", "channel_id", "send_interval", "history_size", "enabled").
		Values(rule.Name, rule.GuildID, rule.TriggerType, rule.ChannelID,
			nullableString(rule.SendInterval), rule.HistorySize, rule.Enabled).
		Suffix(deliveryRuleReturning).
		ToSql()
	if err != nil {
		return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - Create - r.Builder: %w", err)
	}
	out, err := scanDeliveryRule(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - Create - QueryRow: %w", err)
	}
	return out, nil
}

// Update replaces a rule's mutable fields by id.
func (r *DeliveryRulesRepo) Update(ctx context.Context, rule entity.DeliveryRule) (entity.DeliveryRule, error) {
	sql, args, err := r.Builder.
		Update("delivery_rules").
		Set("name", rule.Name).
		Set("guild_id", rule.GuildID).
		Set("trigger_type", rule.TriggerType).
		Set("channel_id", rule.ChannelID).
		Set("send_interval", nullableString(rule.SendInterval)).
		Set("history_size", rule.HistorySize).
		Set("enabled", rule.Enabled).
		Set("updated_at", sq.Expr("NOW()")).
		Where("id = ?", rule.ID).
		Suffix(deliveryRuleReturning).
		ToSql()
	if err != nil {
		return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - Update - r.Builder: %w", err)
	}
	out, err := scanDeliveryRule(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - Update - rule %d not found", rule.ID)
		}
		return entity.DeliveryRule{}, fmt.Errorf("DeliveryRulesRepo - Update - QueryRow: %w", err)
	}
	return out, nil
}

// Delete removes a rule by id.
func (r *DeliveryRulesRepo) Delete(ctx context.Context, id int64) error {
	sql, args, err := r.Builder.Delete("delivery_rules").Where("id = ?", id).ToSql()
	if err != nil {
		return fmt.Errorf("DeliveryRulesRepo - Delete - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("DeliveryRulesRepo - Delete - Exec: %w", err)
	}
	return nil
}

// Count returns the total number of rules.
func (r *DeliveryRulesRepo) Count(ctx context.Context) (int, error) {
	sql, args, err := r.Builder.Select("COUNT(*)").From("delivery_rules").ToSql()
	if err != nil {
		return 0, fmt.Errorf("DeliveryRulesRepo - Count - r.Builder: %w", err)
	}
	var n int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("DeliveryRulesRepo - Count - QueryRow: %w", err)
	}
	return n, nil
}
