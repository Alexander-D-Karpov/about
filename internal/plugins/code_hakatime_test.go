package plugins

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchHakatimeStats(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/current/stats" || r.URL.Query().Get("start") == "" || r.URL.Query().Get("end") == "" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("test-key")) {
			t.Error("missing Hakatime authorization")
		}
		calls++
		if calls > 2 {
			fmt.Fprint(w, `{"totalSeconds":0}`)
			return
		}
		fmt.Fprint(w, `{"totalSeconds":7200,"languages":[{"name":"Go","totalSeconds":7200}],"editors":[{"name":"VS Code/1","totalSeconds":7200}],"platforms":[{"name":"(linux-x86_64)","totalSeconds":7200}]}`)
	}))
	defer srv.Close()

	stats, err := fetchWakatimeStats(context.Background(), srv.Client(), "test-key", srv.URL+"/api/v1")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || stats.Hours7d != 2 || stats.HoursTotal != 2 || len(stats.LangsAllTime) != 1 || stats.LangsAllTime[0].Name != "Go" || len(stats.OSAllTime) != 1 || stats.OSAllTime[0].Name != "Linux" || len(stats.Editors) != 1 || stats.Editors[0].Name != "VS Code" {
		t.Fatalf("unexpected Hakatime stats (calls=%d): %+v", calls, stats)
	}
}
