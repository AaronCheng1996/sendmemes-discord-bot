// Package repo implements application outer layer logic. Each logic group in own file.
package repo

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

//go:generate mockgen -source=contracts.go -destination=../usecase/mocks_repo_test.go -package=usecase_test

type (
	// TranslationRepo -.
	TranslationRepo interface {
		Store(context.Context, entity.Translation) error
		GetHistory(context.Context) ([]entity.Translation, error)
	}

	// TranslationWebAPI -.
	TranslationWebAPI interface {
		Translate(entity.Translation) (entity.Translation, error)
	}

	// ImagesRepo -.
	ImagesRepo interface {
		GetDefault(context.Context) (entity.Image, error)
	}
)
