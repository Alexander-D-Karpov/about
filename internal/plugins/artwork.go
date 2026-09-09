package plugins

import "strings"

// Artwork CDNs encode the rendered size in the URL, so we can ask for a
// variant that matches the box we actually draw into. Lighthouse found ~2 MB of
// cover art fetched at 640px and 300px for 40-100px slots.
//
// Sizes verified against the live CDNs:
//
//	spotify artist  640px 134,940 B | 320px 41,186 B | 160px 13,829 B
//	last.fm         300px  48,073 B | 174px 22,596 B |  64px  6,141 B

// Target widths per render slot, sized at roughly 2-3x the CSS box so the art
// stays crisp on high-DPR phones without shipping a 640px file for a 40px chip.
const (
	artNowPx    = 320 // .music-now__cover  - 130-160px CSS
	artCoverPx  = 256 // .music-cover       - ~100px grid cell
	artRecentPx = 128 // .music-recent__cover - 40px
)

// spotifyVariant is one size of a Spotify image-id prefix family. The last four
// hex characters of the 16-char prefix select the size.
type spotifyVariant struct {
	prefix string
	px     int
}

// Spotify uses a different prefix family for artist images and album art.
var spotifyFamilies = [][]spotifyVariant{
	{ // artist images
		{"ab6761610000f178", 160},
		{"ab67616100005174", 320},
		{"ab6761610000e5eb", 640},
	},
	{ // album art
		{"ab67616d00004851", 64},
		{"ab67616d00001e02", 300},
		{"ab67616d0000b273", 640},
	},
}

// lastfmSizes are the path segments last.fm accepts, smallest first.
var lastfmSizes = []struct {
	seg string
	px  int
}{
	{"64s", 64},
	{"174s", 174},
	{"300x300", 300},
}

// shrinkArtworkURL rewrites a known artwork CDN URL to the smallest variant
// that is still at least targetPx wide. Unknown hosts, unknown prefixes and
// already-smaller variants are returned unchanged - it never upscales, and it
// never guesses at a URL shape it doesn't recognise.
func shrinkArtworkURL(u string, targetPx int) string {
	switch {
	case strings.Contains(u, "i.scdn.co/image/"):
		return shrinkSpotify(u, targetPx)
	case strings.Contains(u, "lastfm-img.freetls.fastly.net/i/u/"),
		strings.Contains(u, "lastfm.freetls.fastly.net/i/u/"):
		return shrinkLastFM(u, targetPx)
	default:
		return u
	}
}

func shrinkSpotify(u string, targetPx int) string {
	for _, family := range spotifyFamilies {
		for _, cur := range family {
			if !strings.Contains(u, cur.prefix) {
				continue
			}
			// Pick the smallest variant that still covers targetPx, but never
			// larger than what we already have.
			best := cur
			for _, cand := range family {
				if cand.px >= targetPx && cand.px < best.px {
					best = cand
				}
			}
			if best.prefix == cur.prefix {
				return u
			}
			return strings.Replace(u, cur.prefix, best.prefix, 1)
		}
	}
	return u
}

func shrinkLastFM(u string, targetPx int) string {
	for _, cur := range lastfmSizes {
		seg := "/i/u/" + cur.seg + "/"
		if !strings.Contains(u, seg) {
			continue
		}
		best := cur
		for _, cand := range lastfmSizes {
			if cand.px >= targetPx && cand.px < best.px {
				best = cand
			}
		}
		if best.seg == cur.seg {
			return u
		}
		return strings.Replace(u, seg, "/i/u/"+best.seg+"/", 1)
	}
	return u
}
