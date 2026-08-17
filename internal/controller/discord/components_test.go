package discord

import "testing"

func TestFullAlbumCustomIDRoundTrip(t *testing.T) {
	for _, id := range []int{0, 1, 7, 12345} {
		got, ok := parseFullAlbumCustomID(fullAlbumCustomID(id))
		if !ok {
			t.Fatalf("parseFullAlbumCustomID(%q) ok=false, want true", fullAlbumCustomID(id))
		}
		if got != id {
			t.Fatalf("round trip id = %d, want %d", got, id)
		}
	}
}

func TestParseFullAlbumCustomID(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantID int
		wantOK bool
	}{
		{name: "valid", input: "fullalbum:42", wantID: 42, wantOK: true},
		{name: "zero", input: "fullalbum:0", wantID: 0, wantOK: true},
		{name: "wrong prefix", input: "rate:42", wantOK: false},
		{name: "no prefix", input: "42", wantOK: false},
		{name: "empty id", input: "fullalbum:", wantOK: false},
		{name: "non-numeric id", input: "fullalbum:abc", wantOK: false},
		{name: "empty", input: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := parseFullAlbumCustomID(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseFullAlbumCustomID(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && id != tt.wantID {
				t.Fatalf("parseFullAlbumCustomID(%q) id = %d, want %d", tt.input, id, tt.wantID)
			}
		})
	}
}

func TestFullAlbumMoreCustomIDRoundTrip(t *testing.T) {
	for _, tc := range []struct{ id, offset int }{{1, 0}, {42, 100}, {9999, 2500}} {
		cid := fullAlbumMoreCustomID(tc.id, tc.offset)
		id, offset, ok := parseFullAlbumMoreCustomID(cid)
		if !ok || id != tc.id || offset != tc.offset {
			t.Fatalf("round trip %q = (%d, %d, %v), want (%d, %d, true)", cid, id, offset, ok, tc.id, tc.offset)
		}
	}
}

func TestParseFullAlbumMoreCustomID(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantID     int
		wantOffset int
		wantOK     bool
	}{
		{name: "valid", input: "fullalbum_more:42:100", wantID: 42, wantOffset: 100, wantOK: true},
		{name: "zero offset", input: "fullalbum_more:42:0", wantID: 42, wantOffset: 0, wantOK: true},
		{name: "other button", input: "fullalbum:42", wantOK: false},
		{name: "missing offset", input: "fullalbum_more:42", wantOK: false},
		{name: "non-numeric id", input: "fullalbum_more:abc:1", wantOK: false},
		{name: "non-numeric offset", input: "fullalbum_more:1:abc", wantOK: false},
		{name: "negative offset", input: "fullalbum_more:1:-5", wantOK: false},
		{name: "unrelated", input: "something:else", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, offset, ok := parseFullAlbumMoreCustomID(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseFullAlbumMoreCustomID(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && (id != tt.wantID || offset != tt.wantOffset) {
				t.Fatalf("parseFullAlbumMoreCustomID(%q) = (%d, %d), want (%d, %d)",
					tt.input, id, offset, tt.wantID, tt.wantOffset)
			}
		})
	}
}

// The two button prefixes share a stem, so the router would silently send every
// continue press to the Full-album handler if either parser got greedy.
func TestFullAlbumButtonPrefixesDoNotCollide(t *testing.T) {
	if _, ok := parseFullAlbumCustomID(fullAlbumMoreCustomID(7, 100)); ok {
		t.Error("parseFullAlbumCustomID accepted a continue-button CustomID")
	}
	if _, _, ok := parseFullAlbumMoreCustomID(fullAlbumCustomID(7)); ok {
		t.Error("parseFullAlbumMoreCustomID accepted a Full-album CustomID")
	}
}
