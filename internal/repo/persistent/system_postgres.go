package persistent

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// SystemRepo performs system-level probes.
type SystemRepo struct {
	*postgres.Postgres
}

// NewSystemRepo creates a system repository.
func NewSystemRepo(pg *postgres.Postgres) *SystemRepo {
	return &SystemRepo{Postgres: pg}
}

// Ping verifies database connectivity.
func (r *SystemRepo) Ping(ctx context.Context) error {
	return r.Pool.Ping(ctx)
}
