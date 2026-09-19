package plugins

import (
	"net/http"
	"testing"
)

// The whole point of the proxy option is that the checker's HTTP client egresses
// through it. Verify the transport is actually wired for the schemes we support,
// that no proxy leaves the client direct, and that a malformed URL degrades to
// direct instead of panicking or silently breaking every check.
func TestServicesProxyWiring(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		wantProxy  bool
		probeURL   string // request URL to resolve the proxy for
		wantTarget string // expected proxy host the transport routes to
	}{
		{name: "empty is direct", url: "", wantProxy: false},
		{name: "http proxy", url: "http://ru-box:8118", wantProxy: true, probeURL: "http://svc.internal/", wantTarget: "ru-box:8118"},
		{name: "socks5 proxy", url: "socks5://ru-box:1080", wantProxy: true, probeURL: "http://svc.internal/", wantTarget: "ru-box:1080"},
		{name: "garbage is direct", url: "://nonsense", wantProxy: false},
		{name: "scheme only is direct", url: "http://", wantProxy: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewServicesPlugin(nil, nil, tc.url)
			tr, _ := p.httpClient.Transport.(*http.Transport)

			if !tc.wantProxy {
				if p.httpClient.Transport != nil {
					t.Fatalf("expected direct client (nil transport), got %#v", p.httpClient.Transport)
				}
				return
			}

			if tr == nil || tr.Proxy == nil {
				t.Fatalf("expected a proxying transport, got %#v", p.httpClient.Transport)
			}
			req, _ := http.NewRequest("GET", tc.probeURL, nil)
			proxyURL, err := tr.Proxy(req)
			if err != nil {
				t.Fatalf("proxy resolve failed: %v", err)
			}
			if proxyURL == nil || proxyURL.Host != tc.wantTarget {
				t.Fatalf("proxy host = %v, want %s", proxyURL, tc.wantTarget)
			}
		})
	}
}
