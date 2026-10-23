package plugins

import (
	"net/http"
	"testing"
)

func TestProxyFuncHonoursNoProxy(t *testing.T) {
	t.Setenv("NO_PROXY", "api.github.com,gitlab.com") // "gitlab.com" covers the domain and its subdomains
	f := proxyFunc("http://proxy.example:3128")

	for host, wantProxy := range map[string]bool{
		"api.github.com":        false,
		"gitlab.com":            false,
		"www.gitlab.com":        false,
		"ws.audioscrobbler.com": true,
	} {
		req, _ := http.NewRequest("GET", "https://"+host+"/x", nil)
		u, err := f(req)
		if err != nil {
			t.Fatal(err)
		}
		if got := u != nil; got != wantProxy {
			t.Errorf("%s: proxied=%v, want %v", host, got, wantProxy)
		}
	}

	if proxyFunc("") != nil {
		t.Error("no proxy URL should mean no proxy")
	}
}
