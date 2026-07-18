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
		chunks := chunkOrdered(l, pool, 2, 1000)

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
		chunks := chunkOrdered(l, pool, 100, 20)

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

	t.Run("oversize single file is skipped", func(t *testing.T) {
		t.Parallel()
		pool := []fileEntry{fe("a", 5), fe("huge", 50), fe("b", 5)}
		chunks := chunkOrdered(l, pool, 100, 20)

		if len(chunks) != 1 {
			t.Fatalf("got %d chunks, want 1", len(chunks))
		}
		if got := chunkNames(chunks[0]); !equalStrings(got, []string{"a", "b"}) {
			t.Fatalf("chunk 0 = %v, want [a b] (huge should be skipped)", got)
		}
	})

	t.Run("empty pool yields no chunks", func(t *testing.T) {
		t.Parallel()
		if chunks := chunkOrdered(l, nil, 10, 100); len(chunks) != 0 {
			t.Fatalf("got %d chunks, want 0", len(chunks))
		}
	})
}
