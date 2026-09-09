package plugins

import "testing"

func TestShrinkArtworkURL_Spotify(t *testing.T) {
	const id = "8a1271f7f32e5202924ebff2"
	cases := []struct {
		name   string
		in     string
		target int
		want   string
	}{
		{
			"640px artist image down to the 160px variant",
			"https://i.scdn.co/image/ab6761610000e5eb" + id, 128,
			"https://i.scdn.co/image/ab6761610000f178" + id,
		},
		{
			"640px artist image down to the 320px variant",
			"https://i.scdn.co/image/ab6761610000e5eb" + id, 300,
			"https://i.scdn.co/image/ab67616100005174" + id,
		},
		{
			"large target keeps the original",
			"https://i.scdn.co/image/ab6761610000e5eb" + id, 640,
			"https://i.scdn.co/image/ab6761610000e5eb" + id,
		},
		{
			// The album family is 64/300/640, so a 128px target must land on
			// 300 - dropping to 64 would be upscaling.
			"album art picks the smallest ADEQUATE size, not the smallest",
			"https://i.scdn.co/image/ab67616d0000b273" + id, 128,
			"https://i.scdn.co/image/ab67616d00001e02" + id,
		},
		{
			"album art can reach the 64px variant when the target is tiny",
			"https://i.scdn.co/image/ab67616d0000b273" + id, 60,
			"https://i.scdn.co/image/ab67616d00004851" + id,
		},
		{
			"never upscales an already-small variant",
			"https://i.scdn.co/image/ab6761610000f178" + id, 640,
			"https://i.scdn.co/image/ab6761610000f178" + id,
		},
		{
			"unknown prefix is left untouched",
			"https://i.scdn.co/image/5419a05563a9ba8cb16fe207199c298f621eef85", 128,
			"https://i.scdn.co/image/5419a05563a9ba8cb16fe207199c298f621eef85",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shrinkArtworkURL(c.in, c.target); got != c.want {
				t.Errorf("shrinkArtworkURL(%q, %d)\n got %q\nwant %q", c.in, c.target, got, c.want)
			}
		})
	}
}

func TestShrinkArtworkURL_LastFM(t *testing.T) {
	const base = "https://lastfm-img.freetls.fastly.net/i/u/"
	const f = "7327bd581388498d9188d75467e535b7.png"
	cases := []struct {
		name   string
		in     string
		target int
		want   string
	}{
		{"300x300 down to 174s", base + "300x300/" + f, 128, base + "174s/" + f},
		{"300x300 down to 64s", base + "300x300/" + f, 60, base + "64s/" + f},
		{"large target keeps 300x300", base + "300x300/" + f, 300, base + "300x300/" + f},
		{"never upscales", base + "64s/" + f, 300, base + "64s/" + f},
		{"other hosts untouched", "https://example.com/i/u/300x300/x.jpg", 64, "https://example.com/i/u/300x300/x.jpg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shrinkArtworkURL(c.in, c.target); got != c.want {
				t.Errorf("shrinkArtworkURL(%q, %d)\n got %q\nwant %q", c.in, c.target, got, c.want)
			}
		})
	}
}

func TestShrinkArtworkURL_PassesThroughJunk(t *testing.T) {
	for _, in := range []string{"", "not a url", "/media/local.png", "https://other.cdn/x.jpg"} {
		if got := shrinkArtworkURL(in, 128); got != in {
			t.Errorf("shrinkArtworkURL(%q) = %q, want unchanged", in, got)
		}
	}
}
