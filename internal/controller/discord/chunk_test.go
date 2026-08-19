package discord

import "testing"

// noopLogger is a zero-cost logger.Interface implementation for pure-function tests.
type noopLogger struct{}

func (noopLogger) Debug(interface{}, ...interface{}) {}
func (noopLogger) Info(string, ...interface{})       {}
func (noopLogger) Warn(string, ...interface{})       {}
func (noopLogger) Error(interface{}, ...interface{}) {}
func (noopLogger) Fatal(interface{}, ...interface{}) {}

func fe(name string, size int) fileEntry {
	return fileEntry{name: name, data: make([]byte, size)}
}

func chunkNames(entries []fileEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.name
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChunkOrdered(t *testing.T) {
	t.Parallel()
	l := noopLogger{}

	t.Run("order preserved across count-capped chunks", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{fe("a", 1), fe("b", 1), fe("c", 1), fe("d", 1), fe("e", 1)}
		chunks, _ := chunkOrdered(l, pool, 2, 1000)

		want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
		if len(chunks) != len(want) {
			t.Fatalf("got %d chunks, want %d", len(chunks), len(want))
		}
		for i, ch := range chunks {
			if got := chunkNames(ch); !equalStrings(got, want[i]) {
				t.Fatalf("chunk %d = %v, want %v", i, got, want[i])
			}
		}
	})

	t.Run("byte cap starts a new chunk", func(t *testing.T) {
		t.Parallel()
		// a+b = 20 exactly fits maxBytes; adding c would exceed it.
		pool := []fileEntry{fe("a", 10), fe("b", 10), fe("c", 10)}
		chunks, _ := chunkOrdered(l, pool, 100, 20)

		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}
		if got := chunkNames(chunks[0]); !equalStrings(got, []string{"a", "b"}) {
			t.Fatalf("chunk 0 = %v, want [a b]", got)
		}
		if got := chunkNames(chunks[1]); !equalStrings(got, []string{"c"}) {
			t.Fatalf("chunk 1 = %v, want [c]", got)
		}
	})

	t.Run("oversize single file is skipped and reported", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{fe("a", 5), fe("huge", 50), fe("b", 5)}
		chunks, oversized := chunkOrdered(l, pool, 100, 20)

		if len(chunks) != 1 {
			t.Fatalf("got %d chunks, want 1", len(chunks))
		}
		if got := chunkNames(chunks[0]); !equalStrings(got, []string{"a", "b"}) {
			t.Fatalf("chunk 0 = %v, want [a b] (huge should be skipped)", got)
		}
		// The caller has to be able to name it: a silently dropped file is
		// indistinguishable from the album not containing it.
		if got := chunkNames(oversized); !equalStrings(got, []string{"huge"}) {
			t.Fatalf("oversized = %v, want [huge]", got)
		}
	})

	t.Run("every file that fits is emitted", func(t *testing.T) {
		t.Parallel()
		// The full-album path relies on this: a pool larger than one message
		// must span several chunks rather than have the remainder dropped.
		pool := make([]fileEntry, 0, 25)
		for i := range 25 {
			pool = append(pool, fe(string(rune('a'+i%26)), 1))
		}
		chunks, oversized := chunkOrdered(l, pool, 10, 1000)

		total := 0
		for _, ch := range chunks {
			total += len(ch)
		}
		if total != len(pool) {
			t.Fatalf("emitted %d of %d files", total, len(pool))
		}
		if len(oversized) != 0 {
			t.Fatalf("oversized = %v, want none", chunkNames(oversized))
		}
	})

	t.Run("empty pool yields no chunks", func(t *testing.T) {
		t.Parallel()
		chunks, oversized := chunkOrdered(l, nil, 10, 100)
		if len(chunks) != 0 {
			t.Fatalf("got %d chunks, want 0", len(chunks))
		}
		if len(oversized) != 0 {
			t.Fatalf("got %d oversized, want 0", len(oversized))
		}
	})
}

// feCover is fe with the album-cover flag set.
func feCover(name string, size int) fileEntry {
	e := fe(name, size)
	e.isCover = true
	return e
}

func TestFitToLimit(t *testing.T) {
	t.Parallel()
	l := noopLogger{}

	t.Run("keeps the whole batch when it fits", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{fe("a", 10), fe("b", 10), fe("c", 10)}
		if got := fitToLimit(l, pool, 10, 1000); len(got) != 3 {
			t.Fatalf("selected %d files, want 3", len(got))
		}
	})

	t.Run("sheds files until the batch fits", func(t *testing.T) {
		t.Parallel()
		// Ten 10-byte files against a 55-byte budget: it has to come down to five.
		pool := make([]fileEntry, 0, 10)
		for i := range 10 {
			pool = append(pool, fe(string(rune('a'+i)), 10))
		}
		got := fitToLimit(l, pool, 10, 55)
		total := 0
		for _, e := range got {
			total += e.size()
		}
		if total > 55 {
			t.Fatalf("selected %d bytes, over the 55-byte budget", total)
		}
		if len(got) == 0 {
			t.Fatal("selected nothing when five files would have fitted")
		}
	})

	// The regression: an oversized cover is pinned and can never be evicted, so
	// the loop used to strip every candidate trying to make room that does not
	// exist and return nothing — even with a pool of perfectly sendable images.
	t.Run("an oversized cover is dropped, not the whole message", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{feCover("cover", 500), fe("a", 10), fe("b", 10), fe("c", 10)}
		got := fitToLimit(l, pool, 10, 100)

		if len(got) == 0 {
			t.Fatal("selected nothing; the three small images should still have gone out")
		}
		for _, e := range got {
			if e.isCover {
				t.Errorf("cover of 500 bytes survived a 100-byte budget")
			}
		}
		if len(got) != 3 {
			t.Errorf("selected %d files, want the 3 that fit", len(got))
		}
	})

	t.Run("a cover that fits still leads the batch", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{feCover("cover", 10), fe("a", 10), fe("b", 10)}
		got := fitToLimit(l, pool, 10, 100)
		if len(got) == 0 || !got[0].isCover {
			t.Fatalf("cover should lead the selection, got %v", chunkNames(got))
		}
	})

	t.Run("nothing fits at all", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{fe("huge", 500), fe("also-huge", 400)}
		if got := fitToLimit(l, pool, 10, 100); len(got) != 0 {
			t.Fatalf("selected %v, want nothing", chunkNames(got))
		}
	})
}
