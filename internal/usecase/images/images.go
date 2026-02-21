package images

import (
	"context"
	"fmt"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// UseCase -.
type UseCase struct {
	repo repo.ImagesRepo
}

// New -.	
func New(r repo.ImagesRepo) *UseCase {
	return &UseCase{repo: r}
}

// GetImage returns the default image.
func (uc *UseCase) GetImage(ctx context.Context) (entity.Image, error) {
	i, err := uc.repo.GetDefault(ctx)
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesUseCase - GetImage - repo.GetDefault: %w", err)
	}
	return i, nil
}
