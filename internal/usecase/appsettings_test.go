package usecase_test

import (
	"context"
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	appsettingsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/appsettings"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSyncIntervalFallback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := NewMockAppSettingsRepo(ctrl)
	uc := appsettingsuc.New(repo, "1h")
	ctx := context.Background()

	// No stored row: env default is used.
	repo.EXPECT().Get(gomock.Any()).Return(entity.AppSettings{}, false, nil)
	got, err := uc.GetSyncInterval(ctx)
	require.NoError(t, err)
	require.Equal(t, "1h", got)

	// Stored value wins over the default.
	repo.EXPECT().Get(gomock.Any()).Return(entity.AppSettings{SyncInterval: "30m"}, true, nil)
	got, err = uc.GetSyncInterval(ctx)
	require.NoError(t, err)
	require.Equal(t, "30m", got)
}

func TestSetSyncIntervalValidation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := NewMockAppSettingsRepo(ctrl)
	uc := appsettingsuc.New(repo, "1h")
	ctx := context.Background()

	// Invalid duration is rejected before persisting.
	_, err := uc.SetSyncInterval(ctx, "not-a-duration")
	require.Error(t, err)

	repo.EXPECT().Upsert(gomock.Any(), entity.AppSettings{SyncInterval: "2h"}).
		Return(entity.AppSettings{SyncInterval: "2h"}, nil)
	out, err := uc.SetSyncInterval(ctx, "2h")
	require.NoError(t, err)
	require.Equal(t, "2h", out.SyncInterval)
}
