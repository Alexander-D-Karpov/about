package main

import "testing"

func TestCacheControlFor(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		// Uploaded media carries an upload timestamp in the filename, so a new
		// file is always a new URL - safe to freeze.
		{"media image is immutable", "/media/uploads/social/20260119110734_beat.png", cacheImmutable},
		{"media jpg is immutable", "/media/uploads/meme/20260112051632_photo.jpg", cacheImmutable},

		// Third-party libs are NOT versioned in the path, so they must stay
		// revalidatable or a Leaflet upgrade would strand cached clients.
		{"libs get the medium tier", "/static/libs/leaflet/leaflet.js", cacheMedium},
		{"libs css gets the medium tier", "/static/libs/leaflet/leaflet.css", cacheMedium},

		// Embedded static assets change only on deploy and have no cache-buster.
		{"static icon", "/static/icons/telegram.svg", cacheShort},
		{"static image", "/static/images/sanspie.jpg", cacheShort},

		// Non-asset paths must never be cached.
		{"html page", "/", cacheNone},
		{"api route", "/api/status", cacheNone},
		{"extensionless", "/static/somefile", cacheNone},

		// The old check used strings.Contains, so these were mis-classified.
		{"dotted dir is not an asset", "/media/my.css.notes/readme", cacheNone},
		{"query string must not fool it", "/api/thing", cacheNone},

		// Formats the old list missed entirely.
		{"webp", "/media/uploads/a/20260101_x.webp", cacheImmutable},
		{"gif", "/static/images/pepe-dance.gif", cacheShort},
		{"woff2", "/static/fonts/x.woff2", cacheShort},
		{"ico", "/static/favicon/favicon.ico", cacheShort},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cacheControlFor(c.path); got != c.want {
				t.Errorf("cacheControlFor(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}
