package webapi

import (
	"strings"
	"testing"
)

// TestPCloudClientTokenParam verifies that the token type selects the correct
// pCloud query parameter: OAuth tokens must be sent as access_token=, session
// tokens (and the default) as auth=.
func TestPCloudClientTokenParam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		tokenType  string
		wantPrefix string
		wantOAuth  bool
	}{
		{name: "oauth", tokenType: "oauth", wantPrefix: "access_token=", wantOAuth: true},
		{name: "oauth case-insensitive", tokenType: "OAuth", wantPrefix: "access_token=", wantOAuth: true},
		{name: "session", tokenType: "session", wantPrefix: "auth=", wantOAuth: false},
		{name: "empty defaults to session", tokenType: "", wantPrefix: "auth=", wantOAuth: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := NewPCloudClient("tok123", tc.tokenType, "", "", "https://api.pcloud.com")
			if got := c.authQuery(); !strings.HasPrefix(got, tc.wantPrefix) {
				t.Fatalf("authQuery() = %q, want prefix %q", got, tc.wantPrefix)
			}
			if c.oauth != tc.wantOAuth {
				t.Fatalf("oauth = %v, want %v", c.oauth, tc.wantOAuth)
			}
		})
	}
}

// TestPublicThumbURL verifies that a share link is turned into a getpubthumb
// URL, and that links without a code parameter yield "" so callers fall back.
func TestPublicThumbURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		publicLink string
		size       string
		want       string
	}{
		{
			name:       "default size",
			publicLink: "https://u.pcloud.link/publink/show?code=XZabc123",
			want:       "https://api.pcloud.com/getpubthumb?code=XZabc123&fileid=42&size=512x512",
		},
		{
			name:       "explicit size",
			publicLink: "https://u.pcloud.link/publink/show?code=XZabc123",
			size:       "128x128",
			want:       "https://api.pcloud.com/getpubthumb?code=XZabc123&fileid=42&size=128x128",
		},
		{
			name:       "no code parameter",
			publicLink: "https://u.pcloud.link/publink/show",
			want:       "",
		},
		{
			name:       "empty link",
			publicLink: "",
			want:       "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := NewPCloudClient("tok123", "session", "", "", "https://api.pcloud.com/")
			if got := c.PublicThumbURL(tc.publicLink, 42, tc.size); got != tc.want {
				t.Fatalf("PublicThumbURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
