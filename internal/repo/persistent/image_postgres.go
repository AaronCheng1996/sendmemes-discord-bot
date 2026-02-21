package persistent

import (
	"context"
	"fmt"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// ImagesRepo -.
type ImagesRepo struct {
	*postgres.Postgres
}

// NewImagesRepo -.
func NewImagesRepo(pg *postgres.Postgres) *ImagesRepo {
	return &ImagesRepo{Postgres: pg}
}

// GetDefault returns the default image (first row by id).
func (r *ImagesRepo) GetDefault(ctx context.Context) (entity.Image, error) {
	sql, args, err := r.Builder.
		Select("id", "url", "source", "guild_id").
		From("images").
		OrderBy("id ASC").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetDefault - r.Builder: %w", err)
	}

	var e entity.Image
	var source, guildID *string
	err = r.Pool.QueryRow(ctx, sql, args...).Scan(&e.ID, &e.URL, &source, &guildID)
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetDefault - QueryRow: %w", err)
	}
	if source != nil {
		e.Source = *source
	}
	if guildID != nil {
		e.GuildID = *guildID
	}
	return e, nil
}
