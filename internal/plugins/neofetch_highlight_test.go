package plugins

import (
	"strings"
	"testing"
)

func TestHighlightNeofetch(t *testing.T) {
	out := "❯ curl akarpov.ru:22828\n" +
		"                  -`                    sanspie@mainserv\n" +
		"                 .o+`                   -------------\n" +
		"                `ooo/                   OS: arch x86_64\n" +
		"               `+oooo:                  Memory: 20.10 GiB / 46.92 GiB (42%)\n" +
		"              `+oooooo:                 CPU Usage: 28.2%\n" +
		"                                        Date & Time: 2026-01-19 16:06:16\n" +
		"<script>"
	got := string(highlightNeofetch(out))
	for _, want := range []string{
		`<span class="nf-prompt">❯</span><span class="nf-cmd"> curl akarpov.ru:22828</span>`,
		`<span class="nf-title">sanspie</span><span class="nf-at">@</span><span class="nf-title">mainserv</span>`,
		`<span class="nf-sep">-------------</span>`,
		`<span class="nf-key">OS</span><span class="nf-colon">:</span> arch x86_64`,
		`<span class="nf-num">20.10 GiB</span> / 46.92 GiB <span class="nf-num">(42%)</span>`,
		`<span class="nf-key">CPU Usage</span><span class="nf-colon">:</span> <span class="nf-num">28.2%</span>`,
		`<span class="nf-key">Date &amp; Time</span>`,
		`<span class="nf-logo">                 .o+`,
		`&lt;script&gt;`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
