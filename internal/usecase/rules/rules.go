// Package rules implements the delivery-rules use case (CRUD + seeding).
package rules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// defaultHistorySize is used when a scheduled rule does not specify one.
const defaultHistorySize = 10

// UseCase manages delivery rules.
type UseCase struct {
	repo repo.DeliveryRulesRepo
}

// New creates a delivery-rules use case.
func New(r repo.DeliveryRulesRepo) *UseCase {
	return &UseCase{repo: r}
}

// List returns all rules ordered by id.
func (uc *UseCase) List(ctx context.Context) ([]entity.DeliveryRule, error) {
	return uc.repo.List(ctx)
}

// ListActiveByTrigger returns enabled rules of the given trigger type.
func (uc *UseCase) ListActiveByTrigger(ctx context.Context, triggerType string) ([]entity.DeliveryRule, error) {
	return uc.repo.ListActiveByTrigger(ctx, triggerType)
}

// Get returns a rule by id.
func (uc *UseCase) Get(ctx context.Context, id int64) (entity.DeliveryRule, error) {
	return uc.repo.GetByID(ctx, id)
}

// Count returns the number of configured rules.
func (uc *UseCase) Count(ctx context.Context) (int, error) {
	return uc.repo.Count(ctx)
}

// normalize validates and fills defaults on a rule prior to persistence.
func normalize(rule *entity.DeliveryRule) error {
	trigger, err := entity.ParseTriggerType(rule.TriggerType)
	if err != nil {
		return err
	}
	rule.TriggerType = trigger

	rule.ChannelID = strings.TrimSpace(rule.ChannelID)
	if rule.ChannelID == "" {
		return fmt.Errorf("channel_id is required")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.GuildID = strings.TrimSpace(rule.GuildID)
	rule.SendInterval = strings.TrimSpace(rule.SendInterval)

	if trigger == entity.TriggerScheduled {
		if _, derr := time.ParseDuration(rule.SendInterval); derr != nil {
			return fmt.Errorf("scheduled rule needs a valid send_interval (e.g. 6h): %w", derr)
		}
		if rule.HistorySize <= 0 {
			rule.HistorySize = defaultHistorySize
		}
	} else {
		// interval / history are meaningless for event-triggered rules.
		rule.SendInterval = ""
		if rule.HistorySize <= 0 {
			rule.HistorySize = defaultHistorySize
		}
	}
	return nil
}

// Create validates and inserts a new rule.
func (uc *UseCase) Create(ctx context.Context, rule entity.DeliveryRule) (entity.DeliveryRule, error) {
	if err := normalize(&rule); err != nil {
		return entity.DeliveryRule{}, err
	}
	return uc.repo.Create(ctx, rule)
}

// Update validates and updates an existing rule.
func (uc *UseCase) Update(ctx context.Context, id int64, rule entity.DeliveryRule) (entity.DeliveryRule, error) {
	if err := normalize(&rule); err != nil {
		return entity.DeliveryRule{}, err
	}
	rule.ID = id
	return uc.repo.Update(ctx, rule)
}

// Delete removes a rule by id.
func (uc *UseCase) Delete(ctx context.Context, id int64) error {
	return uc.repo.Delete(ctx, id)
}

// FirstScheduledChannel returns the channel + history of the first enabled
// scheduled rule (used as a default target for manual triggers / test sends).
func (uc *UseCase) FirstScheduledChannel(ctx context.Context) (string, int, bool, error) {
	scheduled, err := uc.repo.ListActiveByTrigger(ctx, entity.TriggerScheduled)
	if err != nil {
		return "", 0, false, err
	}
	if len(scheduled) == 0 {
		return "", 0, false, nil
	}
	return scheduled[0].ChannelID, scheduled[0].HistorySize, true, nil
}

// EnsureSeeded inserts the provided default rules only when the table is empty,
// so env-derived defaults are created once and can be edited/removed afterwards.
func (uc *UseCase) EnsureSeeded(ctx context.Context, defaults []entity.DeliveryRule) error {
	n, err := uc.repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("RulesUseCase - EnsureSeeded - Count: %w", err)
	}
	if n > 0 {
		return nil
	}
	for _, rule := range defaults {
		if err := normalize(&rule); err != nil {
			// Skip invalid defaults (e.g. scheduled without interval) rather than abort.
			continue
		}
		if _, err := uc.repo.Create(ctx, rule); err != nil {
			return fmt.Errorf("RulesUseCase - EnsureSeeded - Create: %w", err)
		}
	}
	return nil
}
