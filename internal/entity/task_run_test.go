package entity_test

import (
	"strings"
	"testing"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/stretchr/testify/require"
)

func TestTaskRunNormalizeFillsDefaults(t *testing.T) {
	t.Parallel()

	// The single-call shape an ingest client uses: report a finished run
	// without spelling out either timestamp.
	run := entity.TaskRun{Source: "  crawler  ", Task: " SomeArtist ", Status: "SUCCEEDED"}

	require.NoError(t, run.Normalize())
	require.Equal(t, "crawler", run.Source)
	require.Equal(t, "SomeArtist", run.Task)
	require.Equal(t, entity.TaskRunSucceeded, run.Status)
	require.False(t, run.StartedAt.IsZero())
	require.NotNil(t, run.FinishedAt, "a terminal status implies a finish time")
}

func TestTaskRunNormalizeRunningHasNoFinish(t *testing.T) {
	t.Parallel()

	// A client that opens a run and passes a finish time anyway is contradicting
	// itself; the status wins, so the row cannot read as both open and closed.
	finished := time.Now()
	run := entity.TaskRun{Source: "crawler", Status: entity.TaskRunRunning, FinishedAt: &finished}

	require.NoError(t, run.Normalize())
	require.Nil(t, run.FinishedAt)
	require.Zero(t, run.Duration())
}

func TestTaskRunNormalizeRejectsBadInput(t *testing.T) {
	t.Parallel()

	noSource := entity.TaskRun{Status: entity.TaskRunSucceeded}
	require.Error(t, noSource.Normalize())

	badStatus := entity.TaskRun{Source: "crawler", Status: "done"}
	require.Error(t, badStatus.Normalize())
}

func TestTaskRunNormalizeTruncatesOnRuneBoundary(t *testing.T) {
	t.Parallel()

	// Three-byte runes over the 200-byte source cap: a naive cut would land
	// mid-rune and store invalid UTF-8.
	run := entity.TaskRun{Source: strings.Repeat("繪", 100), Status: entity.TaskRunSucceeded}

	require.NoError(t, run.Normalize())
	require.LessOrEqual(t, len(run.Source), 200)
	require.True(t, isValidUTF8(run.Source))
}

func TestTaskRunNormalizeRejectsOversizedDetail(t *testing.T) {
	t.Parallel()

	// Detail is rejected rather than truncated: half a JSON object helps nobody,
	// and a client that outgrew the cap needs to hear about it.
	fat := entity.TaskRun{
		Source: "crawler",
		Status: entity.TaskRunSucceeded,
		Detail: map[string]any{"blob": strings.Repeat("x", 17*1024)},
	}
	err := fat.Normalize()
	require.Error(t, err)
	require.Contains(t, err.Error(), "over the")

	// A payload under the cap goes through untouched.
	ok := entity.TaskRun{
		Source: "crawler",
		Status: entity.TaskRunSucceeded,
		Detail: map[string]any{"downloaded": 14, "skipped": 0},
	}
	require.NoError(t, ok.Normalize())
	require.Equal(t, 14, ok.Detail["downloaded"])
}

func TestTaskRunDuration(t *testing.T) {
	t.Parallel()

	start := time.Now()
	finish := start.Add(90 * time.Second)
	run := entity.TaskRun{StartedAt: start, FinishedAt: &finish}

	require.Equal(t, 90*time.Second, run.Duration())
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
