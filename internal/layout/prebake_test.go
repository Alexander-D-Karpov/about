package layout

import (
	"html/template"
	"strings"
	"testing"
)

func TestViewportBucketsMatchClientGridTransitions(t *testing.T) {
	want := []struct {
		minWidth int
		cols     int
	}{
		{minWidth: 0, cols: 1},
		{minWidth: 364, cols: 2},
		{minWidth: 521, cols: 2},
		{minWidth: 716, cols: 3},
		{minWidth: 781, cols: 2},
		{minWidth: 792, cols: 3},
		{minWidth: 901, cols: 3},
		{minWidth: 1132, cols: 4},
		{minWidth: 1201, cols: 3},
		{minWidth: 1292, cols: 4},
	}

	if len(viewportBuckets) < len(want) {
		t.Fatalf("got %d viewport buckets, want at least %d", len(viewportBuckets), len(want))
	}

	for i, expected := range want {
		got := viewportBuckets[i]
		if got.MinWidth != expected.minWidth || got.Cols != expected.cols {
			t.Fatalf("bucket %d = {minWidth:%d cols:%d}, want {minWidth:%d cols:%d}", i, got.MinWidth, got.Cols, expected.minWidth, expected.cols)
		}
	}
}

func TestExtractLayoutUsesViewportAwareWidthHint(t *testing.T) {
	html := `<section class="projects-section plugin" data-w="3" data-pb-h-1="111" data-pb-h-3="333"></section>`

	narrow := ExtractLayout(html, 0, ViewportBucket{Cols: 1, MosaicWidth: 320}, nil)
	if narrow.Width != 1 || narrow.Height != 122 {
		t.Fatalf("narrow layout = %+v, want width 1 height 122", narrow)
	}

	wide := ExtractLayout(html, 0, ViewportBucket{Cols: 4, MosaicWidth: 1240}, nil)
	if wide.Width != 3 || wide.Height != 348 {
		t.Fatalf("wide layout = %+v, want width 3 height 348", wide)
	}
}

func TestPrebakeAndCSSExposeViewportHeightVars(t *testing.T) {
	plugins := []template.HTML{
		template.HTML(`<section class="profile-section plugin" data-w="2"></section>`),
	}

	prebaked := string(Prebake(plugins, nil)[0])
	if !strings.Contains(prebaked, `data-prebaked="1"`) {
		t.Fatalf("prebaked html missing data-prebaked marker: %s", prebaked)
	}
	if !strings.Contains(prebaked, `--pb-bp0-col:`) || !strings.Contains(prebaked, `--pb-bp0-h:`) {
		t.Fatalf("prebaked html missing viewport vars: %s", prebaked)
	}
	if strings.Contains(prebaked, `min-height:`) {
		t.Fatalf("prebaked html should not bake persistent min-height inline: %s", prebaked)
	}

	css := string(GenerateResponsiveCSS())
	if !strings.Contains(css, `row-gap:0;column-gap:12px;`) {
		t.Fatalf("responsive css should match client grid gaps: %s", css)
	}
	if !strings.Contains(css, `height:var(--pb-bp0-h);`) || !strings.Contains(css, `min-height:var(--pb-bp0-h);`) {
		t.Fatalf("responsive css missing prebaked height vars: %s", css)
	}
}

func TestBiasUpNeverReducesHeight(t *testing.T) {
	for _, raw := range []int{120, 200, 333, 1000, 2600} {
		if got := biasUp(raw, true); got < raw {
			t.Fatalf("measured biasUp(%d) = %d, want >= %d", raw, got, raw)
		}
		if got := biasUp(raw, false); got < raw {
			t.Fatalf("heuristic biasUp(%d) = %d, want >= %d", raw, got, raw)
		}
	}
}

func TestHeuristicBiasIsLargerThanMeasured(t *testing.T) {
	raw := 400
	if biasUp(raw, false) <= biasUp(raw, true) {
		t.Fatalf("heuristic bias %d should exceed measured bias %d",
			biasUp(raw, false), biasUp(raw, true))
	}
}

func TestExtractLayoutBiasesHeightUp(t *testing.T) {
	html := `<section class="tech-section plugin" data-w="1"><div class="plugin__inner">` +
		`<div class="tech-item"></div><div class="tech-item"></div></div></section>`
	bucket := ViewportBucket{Name: "bp0", Cols: 3, MosaicWidth: 900}
	raw := estimateHeightFromHTML("tech", 1, bucket.pluginWidth(1), html)
	got := ExtractLayout(html, 0, bucket, nil).Height
	if got < raw {
		t.Fatalf("ExtractLayout height %d < raw estimate %d (must bias up)", got, raw)
	}
}

type fakeLookup map[string]int // key = plugin|bucket|span

func (f fakeLookup) Get(plugin, bucket string, span int) (int, bool) {
	h, ok := f[plugin+"|"+bucket+"|"+itoaTest(span)]
	return h, ok
}
func itoaTest(n int) string { return string(rune('0' + n)) }

func TestExtractLayoutPrefersStore(t *testing.T) {
	html := `<section class="tech-section plugin" data-w="1"></section>`
	bucket := ViewportBucket{Name: "bp0", Cols: 3, MosaicWidth: 900}
	store := fakeLookup{"tech|bp0|1": 1234}
	got := ExtractLayout(html, 0, bucket, store).Height
	// biased-up measured value, but must be >= the raw stored value
	if got < 1234 {
		t.Fatalf("store height not used: got %d want >= 1234", got)
	}
}

func placementsOverlap(a, b Placement) bool {
	colOverlap := a.Col < b.Col+b.ColSpan && b.Col < a.Col+a.ColSpan
	rowOverlap := a.Row < b.Row+b.RowSpan && b.Row < a.Row+a.RowSpan
	return colOverlap && rowOverlap
}

func TestPackForColsNeverOverlaps(t *testing.T) {
	plugins := []PluginLayout{
		{Name: "webring", Width: 3, Height: 116, Order: 0},
		{Name: "profile", Width: 2, Height: 520, Order: 1},
		{Name: "tech", Width: 1, Height: 640, Order: 2},
		{Name: "social", Width: 1, Height: 300, Order: 3},
		{Name: "health", Width: 2, Height: 260, Order: 4},
		{Name: "bike", Width: 2, Height: 900, Order: 5},
		{Name: "music", Width: 1, Height: 700, Order: 6},
	}
	for _, cols := range []int{1, 2, 3, 4} {
		got := packForCols(plugins, cols)
		for i := 0; i < len(got); i++ {
			for j := i + 1; j < len(got); j++ {
				if placementsOverlap(got[i], got[j]) {
					t.Fatalf("cols=%d: %q and %q overlap: %+v vs %+v",
						cols, got[i].Name, got[j].Name, got[i], got[j])
				}
			}
		}
	}
}

func TestMemeHeightFromImageDimensions(t *testing.T) {
	bucket := ViewportBucket{Name: "bp8", Cols: 3, MosaicWidth: 900}
	tall := `<section class="meme-section plugin" data-w="1" data-pb-imgw="1000" data-pb-imgh="2000"><div class="plugin__inner"><img></div></section>`
	wide := `<section class="meme-section plugin" data-w="1" data-pb-imgw="2000" data-pb-imgh="1000"><div class="plugin__inner"><img></div></section>`
	ht := ExtractLayout(tall, 0, bucket, nil).Height
	hw := ExtractLayout(wide, 0, bucket, nil).Height
	if ht <= hw {
		t.Fatalf("portrait meme height %d should exceed landscape %d", ht, hw)
	}
}

func TestMemeIgnoresStore(t *testing.T) {
	bucket := ViewportBucket{Name: "bp8", Cols: 3, MosaicWidth: 900}
	html := `<section class="meme-section plugin" data-w="1" data-pb-imgw="1000" data-pb-imgh="1000"><div class="plugin__inner"><img></div></section>`
	store := fakeLookup{"meme|bp8|1": 9999}
	h := ExtractLayout(html, 0, bucket, store).Height
	if h >= 9000 {
		t.Fatalf("meme must ignore the stale store height, got %d", h)
	}
}
