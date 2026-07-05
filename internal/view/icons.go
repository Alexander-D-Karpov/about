package view

import (
	"fmt"
	"html/template"
)

var iconInner = map[string]string{
	"music":      `<path d="M9 17V5l10-2v12"/><circle cx="6" cy="17" r="3"/><circle cx="16" cy="15" r="3"/>`,
	"code":       `<path d="M16 18l6-6-6-6"/><path d="M8 6l-6 6 6 6"/>`,
	"games":      `<rect x="2" y="7" width="20" height="11" rx="4"/><path d="M7 11v3M5.5 12.5h3"/><circle cx="16" cy="11.5" r="1"/><circle cx="18.5" cy="14" r="1"/>`,
	"bike":       `<circle cx="6" cy="17" r="4"/><circle cx="18" cy="17" r="4"/><path d="M6 17l4-7h5l3 7M10 10l2-4h3"/>`,
	"host":       `<rect x="3" y="4" width="18" height="6" rx="2"/><rect x="3" y="14" width="18" height="6" rx="2"/><path d="M7 7h.01M7 17h.01"/>`,
	"machine":    `<rect x="2" y="4" width="20" height="14" rx="2"/><path d="M6 9l3 3-3 3M13 15h4M8 22h8"/>`,
	"tech":       `<path d="M12 2l9 5-9 5-9-5 9-5z"/><path d="M3 12l9 5 9-5M3 17l9 5 9-5"/>`,
	"projects":   `<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/>`,
	"heart":      `<path d="M19 14c1.5-1.5 2-3.5 2-5a4 4 0 0 0-7-2.5A4 4 0 0 0 7 6c-1.5 1.5-1 3.5 0 5l5 5 7-2z"/>`,
	"steps":      `<path d="M6.2 13.4c-1.5 0-2.4-1.7-2.4-3.8S4.7 5.2 6.2 5.2 8.6 6.7 8.6 8.8c0 2.9-.9 4.6-2.4 4.6z"/><path d="M4.6 16c0 1.1.6 1.7 1.6 1.7s1.6-.7 1.6-1.7"/><path d="M16.4 11.2c-1.5 0-2.4-1.7-2.4-3.8s.9-4.4 2.4-4.4 2.4 1.5 2.4 3.6c0 2.9-.9 4.6-2.4 4.6z"/><path d="M14.8 13.8c0 1.1.6 1.7 1.6 1.7s1.6-.7 1.6-1.7"/>`,
	"sun":        `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4 12H2M22 12h-2M5 5l1.5 1.5M17.5 17.5L19 19M19 5l-1.5 1.5M6.5 17.5L5 19"/>`,
	"sleep":      `<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>`,
	"clock":      `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
	"battery":    `<rect x="2" y="7" width="16" height="10" rx="2.5"/><path d="M20 10v4"/><path d="M11 9l-2 3h3l-2 3"/>`,
	"chart":      `<path d="M3 3v18h18"/><path d="M7 14l3-3 3 3 4-5"/>`,
	"beaker":     `<path d="M9 3h6M10 3v6l-5 9a2 2 0 0 0 2 3h10a2 2 0 0 0 2-3l-5-9V3"/><path d="M7 15h10"/>`,
	"chat":       `<path d="M21 15a2 2 0 0 1-2 2H8l-4 4V5a2 2 0 0 1 2-2h13a2 2 0 0 1 2 2v10z"/>`,
	"cloud":      `<path d="M18 18a4 4 0 0 0 0-8 6 6 0 0 0-11.7 1.5A3.5 3.5 0 0 0 6.5 18H18z"/>`,
	"terminal":   `<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M6 9l3 3-3 3M13 15h4"/>`,
	"db":         `<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>`,
	"globe":      `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18z"/>`,
	"book":       `<path d="M4 5a2 2 0 0 1 2-2h14v18H6a2 2 0 0 0-2 2V5z"/><path d="M20 17H6"/>`,
	"mail":       `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 7l9 6 9-6"/>`,
	"commit":     `<circle cx="12" cy="12" r="3"/><path d="M3 12h6M15 12h6"/>`,
	"repo":       `<path d="M4 4a2 2 0 0 1 2-2h12v18H6a2 2 0 0 0-2 2V4z"/><path d="M9 2v8l2.5-2L14 10V2"/>`,
	"pr":         `<circle cx="6" cy="6" r="2"/><circle cx="6" cy="18" r="2"/><circle cx="18" cy="18" r="2"/><path d="M6 8v8M18 16V9a3 3 0 0 0-3-3h-3l2-2m0 4l-2-2"/>`,
	"git":        `<circle cx="12" cy="5" r="2"/><circle cx="6" cy="19" r="2"/><circle cx="18" cy="14" r="2"/><path d="M12 7v6M12 13a6 6 0 0 0 6-1M8 18l8-3"/>`,
	"star":       `<path d="M12 3l2.6 5.6 6 .7-4.5 4.1 1.2 6L12 16.8 6.7 19.5l1.2-6L3.4 9.3l6-.7L12 3z"/>`,
	"dice":       `<rect x="3" y="3" width="18" height="18" rx="4"/><circle cx="8" cy="8" r="1.3"/><circle cx="16" cy="8" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="8" cy="16" r="1.3"/><circle cx="16" cy="16" r="1.3"/>`,
	"expand":     `<path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M21 16v3a2 2 0 0 1-2 2h-3M3 16v3a2 2 0 0 0 2 2h3"/>`,
	"shuffle":    `<path d="M16 3h5v5M21 3l-7 7M16 21h5v-5M21 21l-6-6M3 4l5 5M3 20l11-11"/>`,
	"waka":       `<circle cx="12" cy="12" r="9"/><path d="M8 12l2.5 3L16 9"/>`,
	"telegram":   `<path d="M21.5 4.3L2.8 11.4c-.9.4-.9 1.2 0 1.5l4.7 1.5 1.8 5.6c.2.6.5.7 1 .4l2.7-2 4.6 3.4c.6.4 1.2.2 1.4-.6l3.2-15c.2-.9-.4-1.3-1.7-.9zM8.4 13.9l9.4-5.9c.4-.3.8-.1.5.2l-7.8 7-.3 3.3z"/>`,
	"github":     `<path fill="currentColor" stroke="none" fill-rule="evenodd" clip-rule="evenodd" d="M12 2C6.5 2 2 6.6 2 12.3c0 4.5 2.9 8.4 6.8 9.7.5.1.7-.2.7-.5v-1.7c-2.8.6-3.4-1.4-3.4-1.4-.5-1.2-1.1-1.5-1.1-1.5-.9-.6.1-.6.1-.6 1 .1 1.6 1 1.6 1 .9 1.6 2.4 1.1 3 .9.1-.7.4-1.1.6-1.4-2.2-.3-4.6-1.2-4.6-5.1 0-1.1.4-2 1-2.7-.1-.3-.4-1.4.1-2.8 0 0 .9-.3 2.8 1a9.3 9.3 0 0 1 5 0c1.9-1.3 2.8-1 2.8-1 .5 1.4.2 2.5.1 2.8.7.7 1 1.6 1 2.7 0 3.9-2.4 4.8-4.6 5 .4.3.7.9.7 1.9v2.8c0 .3.2.6.7.5A10 10 0 0 0 22 12.3C22 6.6 17.5 2 12 2z"/>`,
	"discord":    `<path fill="currentColor" stroke="none" d="M19.3 5.6A16 16 0 0 0 15.4 4.4l-.2.4a15 15 0 0 1 3.5 1.7 14 14 0 0 0-11.4 0 15 15 0 0 1 3.5-1.7l-.2-.4A16 16 0 0 0 4.7 5.6 16.5 16.5 0 0 0 2.3 17a16 16 0 0 0 4.9 2.4l.6-1.5c-.5-.2-1-.4-1.4-.7l.3-.2a11.4 11.4 0 0 0 10.6 0l.3.2c-.4.3-.9.5-1.4.7l.6 1.5a16 16 0 0 0 4.9-2.4 16.4 16.4 0 0 0-2.4-11.4zM9 14.7c-1 0-1.8-.9-1.8-2s.8-2 1.8-2 1.8.9 1.8 2-.8 2-1.8 2zm6 0c-1 0-1.8-.9-1.8-2s.8-2 1.8-2 1.8.9 1.8 2-.8 2-1.8 2z"/>`,
	"steam":      `<path fill="currentColor" stroke="none" d="M12 2a10 10 0 0 0-10 9.4l5.4 2.2a2.9 2.9 0 0 1 1.6-.5l2.4-3.5v-.1a3.8 3.8 0 1 1 3.8 3.8h-.1l-3.4 2.5a2.9 2.9 0 0 1-5.7.8L2.4 14.9A10 10 0 1 0 12 2zM8.9 17.2l-1.3-.5a2.2 2.2 0 0 0 4-.7 2.2 2.2 0 0 0-2.9-2.7l1.3.6a1.6 1.6 0 1 1-1.1 3zm8.4-7.6a2.5 2.5 0 1 0-2.6 2.5 2.6 2.6 0 0 0 2.6-2.5zm-4.4 0a1.9 1.9 0 1 1 1.9 1.9 1.9 1.9 0 0 1-1.9-1.9z"/>`,
	"vk":         `<path fill="currentColor" stroke="none" d="M13.2 16.4c-5.5 0-8.9-3.8-9-10h2.8c.1 4.6 2.1 6.5 3.7 6.9V6.4h2.6v4c1.6-.2 3.2-1.9 3.8-4h2.6c-.5 2.6-2.2 4.3-3.4 5 1.2.6 3.1 2.1 3.9 4.6h-2.9c-.6-1.9-2-3.3-4-3.5v3.5z"/>`,
	"linkedin":   `<rect x="2" y="2" width="20" height="20" rx="3" fill="currentColor" stroke="none"/><circle cx="6.5" cy="7" r="1.6" fill="#fff" stroke="none"/><rect x="5" y="10" width="3" height="8" fill="#fff" stroke="none"/><path d="M10 10h2.8v1.2c.4-.8 1.5-1.5 2.9-1.5 2.4 0 3.3 1.5 3.3 4V18h-3v-3.6c0-1-.3-1.8-1.4-1.8s-1.6.8-1.6 1.8V18h-3z" fill="#fff" stroke="none"/>`,
	"lastfm":     `<rect x="2" y="2" width="20" height="20" rx="4" fill="currentColor" stroke="none"/><g fill="#fff" stroke="none"><rect x="6" y="13" width="1.8" height="4" rx=".7"/><rect x="9" y="10" width="1.8" height="7" rx=".7"/><rect x="12" y="7.5" width="1.8" height="9.5" rx=".7"/><rect x="15" y="11" width="1.8" height="6" rx=".7"/></g>`,
	"beatleader": `<rect x="2" y="2" width="20" height="20" rx="5" fill="currentColor" stroke="none"/><rect x="7.5" y="7.5" width="9" height="9" rx="2.2" fill="none" stroke="#fff" stroke-width="1.8"/><rect x="10.4" y="10.4" width="3.2" height="3.2" rx="1" fill="#fff" stroke="none"/>`,

	// --- technologies: paste the remaining values verbatim from icons.js (window.SC_ICONS) ---
	// keys to copy: django python fastapi go linux docker postgres java js ts html5 css3 nginx
	// gitlogo react redis clang csharp graphql php flask celery rabbitmq grafana kubernetes
	// bash mysql elastic selenium mongodb sentry ghactions websocket
}

func Icon(name, color string, size int) template.HTML {
	if color == "" {
		color = "currentColor"
	}
	return template.HTML(fmt.Sprintf(
		`<svg width="%d" height="%d" viewBox="0 0 24 24" fill="none" stroke="%s" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" style="color:%s">%s</svg>`,
		size, size, color, color, iconInner[name]))
}
