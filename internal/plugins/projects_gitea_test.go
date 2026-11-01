package plugins

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Alexander-D-Karpov/about/internal/storage"
)

func TestGiteaProjectDates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/sanspie/aego" || r.Header.Get("Authorization") != "token secret" {
			t.Errorf("unexpected Gitea request: %s", r.URL.Path)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"created_at":"2026-09-26T16:31:15+03:00","updated_at":"2026-10-30T00:00:00Z","pushed_at":"2026-10-31T12:00:00Z"}`)
	}))
	defer srv.Close()
	st := storage.New(t.TempDir())
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Wait)
	code := st.GetPluginConfig("code")
	code.Settings["git"] = map[string]interface{}{"sources": []interface{}{map[string]interface{}{
		"type": "gitea", "base_url": srv.URL, "token": "secret",
	}}}
	if err := st.SetPluginConfig("code", code); err != nil {
		t.Fatal(err)
	}
	projects := st.GetPluginConfig("projects")
	projects.Settings["projects"] = []interface{}{map[string]interface{}{"name": "Aego", "github": srv.URL + "/sanspie/aego"}}
	if err := st.SetPluginConfig("projects", projects); err != nil {
		t.Fatal(err)
	}
	if err := NewProjectsPlugin(st, nil).UpdateData(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.GetPluginConfig("projects").Settings["projects"].([]interface{})[0].(map[string]interface{})
	if got["created"] != "2026-09-26" || got["updated"] != "2026-10-31" {
		t.Fatalf("unexpected project dates: %v", got)
	}
}

func TestMatchGiteaRepoRejectsOtherOrigins(t *testing.T) {
	base := "https://git.example.com/git"
	for _, u := range []string{
		"https://evil.example.com/git/user/repo", "http://git.example.com/git/user/repo",
		"https://git.example.com.evil.example/git/user/repo", "https://git.example.com/git/../repo",
		"https://git.example.com/git/user/repo/tree/main", "https://git.example.com/git/user/repo?x=1",
	} {
		if _, _, ok := matchGiteaRepo(base, u); ok {
			t.Errorf("accepted %s", u)
		}
	}
	if owner, repo, ok := matchGiteaRepo(base, "https://git.example.com/git/user/repo.git"); !ok || owner != "user" || repo != "repo" {
		t.Fatalf("valid repository rejected: %q %q %t", owner, repo, ok)
	}
}

func TestGiteaDateFetchDoesNotRedirectToOtherOrigin(t *testing.T) {
	visited := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited = true
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer srv.Close()
	_, _, err := fetchGiteaRepoDates(context.Background(), srv.Client(), gitSourceConfig{BaseURL: srv.URL, Token: "secret"}, "user", "repo")
	if err == nil || visited {
		t.Fatal("cross-origin redirect should not be followed")
	}
}
