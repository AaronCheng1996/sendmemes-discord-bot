package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	rulesuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/rules"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRulesCreateValidation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := NewMockDeliveryRulesRepo(ctrl)
	uc := rulesuc.New(repo)
	ctx := context.Background()

	// scheduled without a valid interval is rejected before hitting the repo.
	_, err := uc.Create(ctx, entity.DeliveryRule{TriggerType: entity.TriggerScheduled, ChannelID: "c1"})
	require.Error(t, err)

	// unknown trigger type is rejected.
	_, err = uc.Create(ctx, entity.DeliveryRule{TriggerType: "bogus", ChannelID: "c1"})
	require.Error(t, err)

	// missing channel is rejected.
	_, err = uc.Create(ctx, entity.DeliveryRule{TriggerType: entity.TriggerNewAlbum})
	require.Error(t, err)

	// valid event rule reaches the repo with defaults filled in.
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, r entity.DeliveryRule) (entity.DeliveryRule, error) {
			require.Equal(t, entity.TriggerNewAlbum, r.TriggerType)
			require.Equal(t, 10, r.HistorySize) // default
			require.Empty(t, r.SendInterval)    // cleared for event rules
			r.ID = 1
			return r, nil
		})
	out, err := uc.Create(ctx, entity.DeliveryRule{TriggerType: entity.TriggerNewAlbum, ChannelID: "c1"})
	require.NoError(t, err)
	require.Equal(t, int64(1), out.ID)
}

func TestRulesGetDecoratesScheduledRule(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := NewMockDeliveryRulesRepo(ctrl)
	uc := rulesuc.New(repo)
	ctx := context.Background()

	repo.EXPECT().GetByID(gomock.Any(), int64(1)).Return(entity.DeliveryRule{
		ID: 1, TriggerType: entity.TriggerScheduled, SendInterval: "6h", ChannelID: "c1",
	}, nil)

	before := time.Now()
	out, err := uc.Get(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, out.NextRunAt)
	require.True(t, out.NextRunAt.After(before))
	require.Equal(t, "every 6h", out.ScheduleDescription)
}

func TestRulesGetLeavesEventRuleUndecorated(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := NewMockDeliveryRulesRepo(ctrl)
	uc := rulesuc.New(repo)
	ctx := context.Background()

	repo.EXPECT().GetByID(gomock.Any(), int64(2)).Return(entity.DeliveryRule{
		ID: 2, TriggerType: entity.TriggerNewAlbum, ChannelID: "c1",
	}, nil)

	out, err := uc.Get(ctx, 2)
	require.NoError(t, err)
	require.Nil(t, out.NextRunAt)
	require.Empty(t, out.ScheduleDescription)
}

func TestRulesListDecoratesEachRule(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := NewMockDeliveryRulesRepo(ctrl)
	uc := rulesuc.New(repo)
	ctx := context.Background()

	repo.EXPECT().List(gomock.Any()).Return([]entity.DeliveryRule{
		{ID: 1, TriggerType: entity.TriggerScheduled, SendInterval: "0 9 * * *", ChannelID: "c1"},
		{ID: 2, TriggerType: entity.TriggerNewFiles, ChannelID: "c2"},
	}, nil)

	out, err := uc.List(ctx)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.NotNil(t, out[0].NextRunAt)
	require.Equal(t, "at 09:00 every day", out[0].ScheduleDescription)
	require.Nil(t, out[1].NextRunAt)
	require.Empty(t, out[1].ScheduleDescription)
}

func TestRulesEnsureSeeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Non-empty table: seeding is a no-op (no Create calls).
	ctrl := gomock.NewController(t)
	repo := NewMockDeliveryRulesRepo(ctrl)
	repo.EXPECT().Count(gomock.Any()).Return(2, nil)
	require.NoError(t, rulesuc.New(repo).EnsureSeeded(ctx, []entity.DeliveryRule{
		{TriggerType: entity.TriggerNewAlbum, ChannelID: "c"},
	}))

	// Empty table: each valid default is inserted.
	ctrl2 := gomock.NewController(t)
	repo2 := NewMockDeliveryRulesRepo(ctrl2)
	repo2.EXPECT().Count(gomock.Any()).Return(0, nil)
	repo2.EXPECT().Create(gomock.Any(), gomock.Any()).Return(entity.DeliveryRule{ID: 1}, nil)
	require.NoError(t, rulesuc.New(repo2).EnsureSeeded(ctx, []entity.DeliveryRule{
		{TriggerType: entity.TriggerNewFiles, ChannelID: "c"},
	}))
}
