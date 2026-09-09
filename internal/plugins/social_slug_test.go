package plugins

import "testing"

func TestSocialIconSlug(t *testing.T) {
	cases := []struct {
		name  string
		icon  string
		label string
		want  string
	}{
		{"plain icon name", "ctftime", "CTFtime", "ctftime"},
		{"icon wins over label", "email", "Email", "email"},
		{"falls back to label when icon empty", "", "BeatLeader", "beatleader"},
		{"label with a dot", "", "Last.fm", "lastfm"},
		{"label with spaces", "", "Tier Rankings", "tier-rankings"},
		{"filename gets extension stripped", "github.svg", "GitHub", "github"},
		{"absolute path is reduced to basename", "/static/icons/vk.svg", "VK", "vk"},
		{"remote url is reduced to basename", "https://cdn.example.com/a/steam.png", "Steam", "steam"},
		{"both empty", "", "", ""},
		{"uppercase is normalised", "CodeWars", "CodeWars", "codewars"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := socialIconSlug(c.icon, c.label); got != c.want {
				t.Errorf("socialIconSlug(%q, %q) = %q, want %q", c.icon, c.label, got, c.want)
			}
		})
	}
}
