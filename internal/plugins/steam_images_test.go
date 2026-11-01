package plugins

import (
	"strings"
	"testing"
)

func TestSteamCurrentGameImages(t *testing.T) {
	hero, fallback := steamCurrentGameImages("427520")
	if !strings.HasSuffix(hero, "/427520/library_hero.jpg") || !strings.HasSuffix(fallback, "/427520/header.jpg") {
		t.Fatalf("unexpected Steam image URLs %q %q", hero, fallback)
	}
	if hero, fallback := steamCurrentGameImages("../x"); hero != "" || fallback != "" {
		t.Fatal("invalid game ID should not produce URLs")
	}
}

func TestSteamGameViewsUseHighResolutionArt(t *testing.T) {
	views := steamGameViews([]SteamGame{{AppID: 427520, Name: "Factorio", ImgIconURL: "iconhash"}}, 1)
	if len(views) != 1 || !strings.HasSuffix(views[0].Icon, "/427520/library_600x900.jpg") || !strings.HasSuffix(views[0].FallbackIcon, "/427520/iconhash.jpg") {
		t.Fatalf("unexpected game artwork: %+v", views)
	}
}
