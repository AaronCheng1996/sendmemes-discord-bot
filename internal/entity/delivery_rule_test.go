package entity_test

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/stretchr/testify/require"
)

func TestParseTriggerType(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"new_album", "new_files", "scheduled"} {
		got, err := entity.ParseTriggerType(v)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err := entity.ParseTriggerType("  scheduled  ")
	require.NoError(t, err)
	require.Equal(t, entity.TriggerScheduled, got)

	_, err = entity.ParseTriggerType("bogus")
	require.Error(t, err)

	_, err = entity.ParseTriggerType("")
	require.Error(t, err)
}

func TestSyncEventTriggerType(t *testing.T) {
	t.Parallel()

	require.Equal(t, entity.TriggerNewAlbum, entity.SyncEventTriggerType(entity.SyncEventAlbumCreated))
	require.Equal(t, entity.TriggerNewFiles, entity.SyncEventTriggerType(entity.SyncEventFilesAdded))
}
