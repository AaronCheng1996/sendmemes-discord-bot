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
