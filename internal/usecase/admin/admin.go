package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
)

// UseCase provides admin CRUD and settings operations.
type UseCase struct {
	albums   repo.AlbumsRepo
	images   repo.ImagesRepo
	settings usecase.Settings
}

// New creates admin usecase.
func New(albums repo.AlbumsRepo, images repo.ImagesRepo, settings usecase.Settings) *UseCase {
	return &UseCase{albums: albums, images: images, settings: settings}
}

func (uc *UseCase) ListAlbums(ctx context.Context, offset, limit int) ([]entity.Album, error) {
	return uc.albums.List(ctx, offset, limit)
}

func (uc *UseCase) GetAlbum(ctx context.Context, id int) (entity.Album, error) {
	return uc.albums.GetByID(ctx, id)
}

func (uc *UseCase) CreateAlbum(ctx context.Context, name string) (entity.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Album{}, fmt.Errorf("album name is required")
	}
	return uc.albums.Create(ctx, name)
}

func (uc *UseCase) UpdateAlbum(ctx context.Context, id int, name string) (entity.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Album{}, fmt.Errorf("album name is required")
	}
	return uc.albums.Update(ctx, id, name)
}

func (uc *UseCase) DeleteAlbum(ctx context.Context, id int) error {
	return uc.albums.Delete(ctx, id)
}

func (uc *UseCase) ListImages(ctx context.Context, albumID, offset, limit int) ([]entity.Image, error) {
	return uc.images.List(ctx, albumID, offset, limit)
}

func (uc *UseCase) GetImage(ctx context.Context, id int) (entity.Image, error) {
	return uc.images.GetByID(ctx, id)
}

func (uc *UseCase) CreateImage(ctx context.Context, img entity.Image) (entity.Image, error) {
	if strings.TrimSpace(img.URL) == "" {
		return entity.Image{}, fmt.Errorf("image url is required")
	}
	return uc.images.Insert(ctx, img)
}

func (uc *UseCase) UpdateImage(ctx context.Context, img entity.Image) (entity.Image, error) {
	if img.ID <= 0 {
		return entity.Image{}, fmt.Errorf("image id is required")
	}
	if strings.TrimSpace(img.URL) == "" {
		return entity.Image{}, fmt.Errorf("image url is required")
	}
	return uc.images.Update(ctx, img)
}

func (uc *UseCase) DeleteImage(ctx context.Context, id int) error {
	return uc.images.Delete(ctx, id)
}

func (uc *UseCase) GetEffectiveSchedule(ctx context.Context, guildID string) (entity.EffectiveScheduleSettings, error) {
	return uc.settings.GetEffectiveSchedule(ctx, guildID)
}

func (uc *UseCase) UpsertSchedule(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error) {
	return uc.settings.UpsertSchedule(ctx, cfg)
}
