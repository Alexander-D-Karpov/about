package plugins

import (
	"html/template"
	"regexp"
	"strings"
)

// Pasted neofetch/fastfetch output is plain text (the ANSI colours are lost on
// copy), so the colours are put back here the way the tools draw them: the
// distro logo and the "Key:" labels in the logo colour, user@host in bold with
// a plain "@", the shell prompt line in green, and - as fastfetch does - the
// used amount and the percentage of each "used / total" reading highlighted.

var (
	neoUserHostRe = regexp.MustCompile(`\s{2,}([A-Za-z0-9._-]+@[A-Za-z0-9._-]+)\s*$`)
	neoKeyRe      = regexp.MustCompile(`^([A-Za-z][^:]{0,40}?):(\s.*|)$`)
	neoUsedRe     = regexp.MustCompile(`^(\s*)([\d.]+\s?[KMGTP]?i?B)(\s*/\s*)`)
	neoPctRe      = regexp.MustCompile(`\(\d+(?:\.\d+)?%\)`)
	neoBarePctRe  = regexp.MustCompile(`^(\s*)(\d+(?:\.\d+)?%)`)
)

func neoSpan(class, text string) string {
	return `<span class="` + class + `">` + template.HTMLEscapeString(text) + `</span>`
}

// highlightNeofetch turns raw fetch output into coloured, escaped HTML.
func highlightNeofetch(out string) template.HTML {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")

	// The info column starts where user@host does; every line is split there
	// into logo (left) and info (right).
	infoCol := -1
	for _, l := range lines {
		if m := neoUserHostRe.FindStringSubmatchIndex(l); m != nil {
			infoCol = len([]rune(l[:m[2]]))
			break
		}
	}

	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if rest, ok := strings.CutPrefix(l, "❯"); ok {
			b.WriteString(neoSpan("nf-prompt", "❯"))
			b.WriteString(neoSpan("nf-cmd", rest))
			continue
		}
		r := []rune(l)
		if infoCol < 0 || len(r) <= infoCol {
			b.WriteString(neoSpan("nf-logo", l))
			continue
		}
		logo, info := string(r[:infoCol]), string(r[infoCol:])
		if strings.TrimSpace(logo) != "" {
			b.WriteString(neoSpan("nf-logo", logo))
		} else {
			b.WriteString(logo)
		}
		b.WriteString(highlightNeofetchInfo(info))
	}
	return template.HTML(b.String())
}

func highlightNeofetchInfo(info string) string {
	trimmed := strings.TrimSpace(info)
	switch {
	case trimmed == "":
		return template.HTMLEscapeString(info)
	case strings.Trim(trimmed, "-") == "":
		return neoSpan("nf-sep", info)
	}
	if user, host, ok := strings.Cut(trimmed, "@"); ok && !strings.ContainsAny(trimmed, " :") {
		return neoSpan("nf-title", user) + neoSpan("nf-at", "@") + neoSpan("nf-title", host)
	}
	m := neoKeyRe.FindStringSubmatch(info)
	if m == nil {
		return template.HTMLEscapeString(info)
	}
	return neoSpan("nf-key", m[1]) + neoSpan("nf-colon", ":") + highlightNeofetchValue(m[1], m[2])
}

func highlightNeofetchValue(key, v string) string {
	var b strings.Builder
	rest := v
	if m := neoUsedRe.FindStringSubmatch(rest); m != nil {
		b.WriteString(template.HTMLEscapeString(m[1]))
		b.WriteString(neoSpan("nf-num", m[2]))
		b.WriteString(template.HTMLEscapeString(m[3]))
		rest = rest[len(m[0]):]
	} else if strings.Contains(strings.ToLower(key), "usage") {
		if m := neoBarePctRe.FindStringSubmatch(rest); m != nil {
			b.WriteString(template.HTMLEscapeString(m[1]))
			b.WriteString(neoSpan("nf-num", m[2]))
			rest = rest[len(m[0]):]
		}
	}
	last := 0
	for _, loc := range neoPctRe.FindAllStringIndex(rest, -1) {
		b.WriteString(template.HTMLEscapeString(rest[last:loc[0]]))
		b.WriteString(neoSpan("nf-num", rest[loc[0]:loc[1]]))
		last = loc[1]
	}
	b.WriteString(template.HTMLEscapeString(rest[last:]))
	return b.String()
}
