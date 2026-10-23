package plugins

import "testing"

func song(name, author string) AkarpovMusicTrack {
	s := AkarpovMusicTrack{Name: name}
	s.Authors = append(s.Authors, struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		ImageCropped string `json:"image_cropped"`
	}{Name: author})
	return s
}

func TestBestSongMatchPrefersTheRequestedOriginal(t *testing.T) {
	songs := []AkarpovMusicTrack{
		song("Better Now (Remix)", "Post Malone"),
		song("Better Now", "Some Cover Band"),
		song("Better Now", "Post Malone"),
		song("Better Now - Live", "Post Malone"),
	}
	if got := bestSongMatch(songs, "Post Malone", "Better Now", ""); got.Name != "Better Now" || got.Authors[0].Name != "Post Malone" {
		t.Fatalf("artist+track picked %q by %q", got.Name, got.Authors[0].Name)
	}
	if got := bestSongMatch(songs, "", "", "Post Malone Better Now"); got.Name != "Better Now" || got.Authors[0].Name != "Post Malone" {
		t.Fatalf("query picked %q by %q", got.Name, got.Authors[0].Name)
	}
	if got := bestSongMatch(songs, "", "", "Better Now remix"); got.Name != "Better Now (Remix)" {
		t.Fatalf("asking for the remix picked %q", got.Name)
	}
}
