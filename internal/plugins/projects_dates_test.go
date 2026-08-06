package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/Alexander-D-Karpov/about/internal/storage"
)

// Verifies the projects plugin renders created/updated dates (normalized) and a
// footer, and that undated projects render without one.
func TestProjectsRendersDates(t *testing.T) {
	st := storage.New(t.TempDir())
	cfg := st.GetPluginConfig("projects")
	cfg.Settings["projects"] = []interface{}{
		map[string]interface{}{
			"name":        "dated project",
			"description": "has both dates",
			"github":      "https://github.com/x/y",
			"created":     "2025-08-17",           // ISO
			"updated":     "2026-03-02T10:00:00Z", // RFC3339 -> should normalize
		},
		map[string]interface{}{
			"name":        "undated project",
			"description": "no dates",
		},
	}
	if err := st.SetPluginConfig("projects", cfg); err != nil {
		t.Fatal(err)
	}

	p := NewProjectsPlugin(st, nil)
	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"project-footer", "Created 17 Aug 2025", "Updated 02 Mar 2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}
	// The undated project must not emit a footer of its own; there should be
	// exactly one footer (from the dated project).
	if n := strings.Count(out, "project-footer"); n != 1 {
		t.Errorf("expected exactly 1 project-footer, got %d", n)
	}
}

// Projects must render most-recently-updated first, undated last.
func TestProjectsSortedByUpdated(t *testing.T) {
	st := storage.New(t.TempDir())
	cfg := st.GetPluginConfig("projects")
	cfg.Settings["projects"] = []interface{}{
		map[string]interface{}{"name": "older", "updated": "2023-01-01"},
		map[string]interface{}{"name": "undated"},
		map[string]interface{}{"name": "newest", "updated": "2026-05-05"},
	}
	if err := st.SetPluginConfig("projects", cfg); err != nil {
		t.Fatal(err)
	}
	out, err := NewProjectsPlugin(st, nil).Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	iNew := strings.Index(out, "newest")
	iOld := strings.Index(out, "older")
	iUn := strings.Index(out, "undated")
	if !(iNew >= 0 && iNew < iOld && iOld < iUn) {
		t.Errorf("wrong order: newest=%d older=%d undated=%d (want newest<older<undated)", iNew, iOld, iUn)
	}
}

func TestParseGitHubRepo(t *testing.T) {
	cases := []struct{ in, o, r, p string }{
		{"https://github.com/Alexander-D-Karpov/about", "Alexander-D-Karpov", "about", ""},
		{"https://github.com/Alexander-D-Karpov/about.git", "Alexander-D-Karpov", "about", ""},
		{"https://github.com/Alexander-D-Karpov/akarpov/tree/main/akarpov/music", "Alexander-D-Karpov", "akarpov", "akarpov/music"},
		{"https://github.com/Alexander-D-Karpov/scripts/tree/master/stream", "Alexander-D-Karpov", "scripts", "stream"},
		{"https://sharoboom-livny.ru/", "", "", ""},
	}
	for _, c := range cases {
		o, r, p := parseGitHubRepo(c.in)
		if o != c.o || r != c.r || p != c.p {
			t.Errorf("parseGitHubRepo(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, o, r, p, c.o, c.r, c.p)
		}
	}
}

func TestFmtProjectDate(t *testing.T) {
	cases := map[string]string{
		"2025-08-17":           "17 Aug 2025",
		"2026-03-02T10:00:00Z": "02 Mar 2026",
		"17 Aug 2025":          "17 Aug 2025",
		"":                     "",
		"garbage":              "garbage", // returned as-is, never dropped
	}
	for in, want := range cases {
		if got := fmtProjectDate(in); got != want {
			t.Errorf("fmtProjectDate(%q) = %q, want %q", in, got, want)
		}
	}
}
