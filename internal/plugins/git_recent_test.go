package plugins

import (
	"testing"
	"time"
)

func TestMergeRecentRepoMirrors(t *testing.T) {
	now := time.Now()
	in := []GitRecentRepo{
		{Name: "ib-lab1", URL: "https://git.example/ib-lab1", Commits: 4, Source: "gitea", SourceColor: "#1", LastActive: now},
		{Name: "aego", URL: "https://git.example/aego", Commits: 50, Source: "gitea", LastActive: now.Add(-time.Hour)},
		{Name: "IB-lab1", URL: "https://github.com/x/ib-lab1", Commits: 4, Stars: 2, Source: "github", SourceColor: "#2",
			Languages: []CodeLangStat{{Name: "Python"}}, LastActive: now.Add(-2 * time.Hour)},
		{Name: "aego", URL: "https://git.example/aego", Source: "gitea"}, // same URL twice: dropped
	}
	out := mergeRecentRepoMirrors(in)
	if len(out) != 2 {
		t.Fatalf("got %d repos, want 2: %+v", len(out), out)
	}
	m := out[0]
	if m.Name != "ib-lab1" || len(m.Mirrors) != 2 {
		t.Fatalf("first repo = %+v, want ib-lab1 with 2 mirrors", m)
	}
	if m.URL != "https://github.com/x/ib-lab1" || m.Stars != 2 || len(m.Languages) != 1 || !m.LastActive.Equal(now) {
		t.Fatalf("merged repo fields wrong: %+v", m)
	}
	if out[1].Name != "aego" || len(out[1].Mirrors) != 1 {
		t.Fatalf("second repo = %+v", out[1])
	}

	// Merging an already-merged list keeps every host.
	again := mergeRecentRepoMirrors(out)
	if len(again[0].Mirrors) != 2 {
		t.Fatalf("re-merge lost hosts: %+v", again[0].Mirrors)
	}
}
