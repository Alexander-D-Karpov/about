package plugins

import (
	"context"
	"errors"
	"html/template"
	"testing"

	"github.com/Alexander-D-Karpov/about/internal/stream"
)

// fakePlugin is a minimal Plugin whose Render output we control.
type fakePlugin struct {
	name string
	html string
	err  error
}

func (f *fakePlugin) Name() string { return f.name }
func (f *fakePlugin) Render(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.html, nil
}
func (f *fakePlugin) RenderText(context.Context) (string, error) { return "", nil }
func (f *fakePlugin) UpdateData(context.Context) error           { return nil }
func (f *fakePlugin) GetSettings() map[string]interface{}        { return nil }
func (f *fakePlugin) SetSettings(map[string]interface{}) error   { return nil }

func newTestManager(ps ...Plugin) *Manager {
	m := &Manager{
		plugins:       map[string]Plugin{},
		renderedCache: map[string]template.HTML{},
	}
	for _, p := range ps {
		m.plugins[p.Name()] = p
	}
	return m
}

// renderSelective must RETURN the freshly rendered HTML, not just warm the
// cache. preRenderPluginsSelective broadcasts those results as
// "plugin_rendered" frames; when it returned nothing, clients had no way to
// see new content and the frontend fell back to a full page reload on every
// background update cycle.
func TestRenderSelectiveReturnsRenderedHTML(t *testing.T) {
	m := newTestManager(
		&fakePlugin{name: "bike", html: "<section class=\"bike-section\">bike</section>"},
		&fakePlugin{name: "music", html: "<section class=\"music-section\">music</section>"},
	)

	got := m.renderSelective(context.Background(), []string{"bike", "music"})
	if len(got) != 2 {
		t.Fatalf("expected 2 rendered plugins to broadcast, got %d: %+v", len(got), got)
	}

	byName := map[string]string{}
	for _, r := range got {
		byName[r.name] = r.html
	}
	if byName["bike"] != "<section class=\"bike-section\">bike</section>" {
		t.Errorf("bike html = %q", byName["bike"])
	}
	if byName["music"] == "" {
		t.Error("music html missing")
	}

	// The cache must still be populated for fresh page loads.
	if m.renderedCache["bike"] == "" {
		t.Error("renderedCache not populated for bike")
	}
}

func TestRenderSelectiveSkipsFailuresAndEmpty(t *testing.T) {
	m := newTestManager(
		&fakePlugin{name: "bike", html: "<b>ok</b>"},
		&fakePlugin{name: "music", err: errors.New("boom")},
		&fakePlugin{name: "code", html: ""},
	)

	got := m.renderSelective(context.Background(), []string{"bike", "music", "code"})
	if len(got) != 1 || got[0].name != "bike" {
		t.Fatalf("expected only bike to be broadcastable, got %+v", got)
	}
}

// These four are deliberately excluded - they have their own live update paths.
func TestRenderSelectiveSkipsSelfUpdatingPlugins(t *testing.T) {
	m := newTestManager(
		&fakePlugin{name: "meme", html: "<b>m</b>"},
		&fakePlugin{name: "info", html: "<b>i</b>"},
		&fakePlugin{name: "visitors", html: "<b>v</b>"},
		&fakePlugin{name: "services", html: "<b>s</b>"},
	)

	got := m.renderSelective(context.Background(), []string{"meme", "info", "visitors", "services"})
	if len(got) != 0 {
		t.Fatalf("expected none of the self-updating plugins to be re-broadcast, got %+v", got)
	}
}

func TestRenderSelectiveEmptyInput(t *testing.T) {
	m := newTestManager()
	if got := m.renderSelective(context.Background(), nil); len(got) != 0 {
		t.Fatalf("expected no results for empty input, got %+v", got)
	}
}

// broadcastRenderedIfChanged must suppress identical re-renders. The music
// plugin re-renders every 20s off the Last.fm ticker and was pushing the same
// ~25KB payload each time; clients replace that section and re-pack the mosaic
// on every frame, so the page visibly rebuilt three times a minute.
//
// Uses a real hub: stream.New() has a 1024-buffered broadcast channel, so
// Broadcast is safe without Run().
func TestBroadcastRenderedIfChangedSuppressesIdentical(t *testing.T) {
	m := newTestManager()
	m.hub = stream.New()

	if !m.broadcastRenderedIfChanged("music", "<section>same</section>") {
		t.Fatal("first render should broadcast")
	}
	if m.broadcastRenderedIfChanged("music", "<section>same</section>") {
		t.Error("identical render should be suppressed")
	}
	if m.broadcastRenderedIfChanged("music", "<section>same</section>") {
		t.Error("still identical - should stay suppressed")
	}
	if !m.broadcastRenderedIfChanged("music", "<section>different</section>") {
		t.Error("changed render must broadcast")
	}
	// Hashes are per-plugin, so the same markup under another name still sends.
	if !m.broadcastRenderedIfChanged("steam", "<section>same</section>") {
		t.Error("per-plugin hashes must be independent")
	}
}

func TestBroadcastRenderedIfChangedIgnoresEmptyAndNilHub(t *testing.T) {
	m := newTestManager()
	m.hub = stream.New()
	if m.broadcastRenderedIfChanged("music", "") {
		t.Error("empty render should never broadcast")
	}

	noHub := newTestManager()
	if noHub.broadcastRenderedIfChanged("music", "<b>x</b>") {
		t.Error("must not broadcast without a hub")
	}
}

// A render whose only differences are volatile (progress width, elapsed
// counter, "N ago" labels) must not trigger a broadcast. These two snippets
// are the exact lines that differed between two consecutive live music
// payloads captured from the running site.
func TestVolatileOnlyChangesAreSuppressed(t *testing.T) {
	a := `<div class="music-progress__fill" data-music-fill style="width:62%"></div>
<span data-music-elapsed>2:54</span>
<span class="music-recent__time">9m ago</span>
<span class="music-recent__time">15m ago</span>
<div class="track">Real Track Name</div>`

	b := `<div class="music-progress__fill" data-music-fill style="width:69%"></div>
<span data-music-elapsed>3:14</span>
<span class="music-recent__time">10m ago</span>
<span class="music-recent__time">16m ago</span>
<div class="track">Real Track Name</div>`

	if normalizeForDedup(a) != normalizeForDedup(b) {
		t.Errorf("volatile-only diff should normalise equal:\n a=%q\n b=%q",
			normalizeForDedup(a), normalizeForDedup(b))
	}

	m := newTestManager()
	m.hub = stream.New()
	if !m.broadcastRenderedIfChanged("music", a) {
		t.Fatal("first render must broadcast")
	}
	if m.broadcastRenderedIfChanged("music", b) {
		t.Error("volatile-only change must NOT re-broadcast")
	}
}

// A real content change must still get through immediately.
func TestRealChangeStillBroadcasts(t *testing.T) {
	a := `<span data-music-elapsed>2:54</span><div class="track">Song A</div>`
	b := `<span data-music-elapsed>3:14</span><div class="track">Song B</div>`

	if normalizeForDedup(a) == normalizeForDedup(b) {
		t.Fatal("different tracks must not normalise equal")
	}

	m := newTestManager()
	m.hub = stream.New()
	m.broadcastRenderedIfChanged("music", a)
	if !m.broadcastRenderedIfChanged("music", b) {
		t.Error("a new track must broadcast even though the timer also changed")
	}
}

// "Last played 14m ago" has its digits mid-sentence rather than right after
// the tag, which an element-anchored pattern missed - and that one line kept
// the whole 27KB music section re-broadcasting every minute.
func TestPrefixedRelativeTimeIsNormalised(t *testing.T) {
	a := `<div class="music-now__status" data-music-status>Last played 14m ago</div>`
	b := `<div class="music-now__status" data-music-status>Last played 15m ago</div>`
	if normalizeForDedup(a) != normalizeForDedup(b) {
		t.Errorf("prefixed relative time should normalise equal:\n a=%q\n b=%q",
			normalizeForDedup(a), normalizeForDedup(b))
	}
}
