package layout

import (
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func reMustCompile(p string) *regexp.Regexp { return regexp.MustCompile(p) }

// bucketFor resolves the bucket a browser lands in at this width, the way the
// stacked `@media(min-width:...)` rules do: the last one whose MinWidth fits.
func bucketFor(width int) ViewportBucket {
	b := viewportBuckets[0]
	for _, cand := range viewportBuckets {
		if cand.MinWidth <= width {
			b = cand
		}
	}
	return b
}

// Whatever the bucket boundaries are, the bucket a viewport resolves to must
// agree with the client about how many columns the grid has — the baked
// grid-column values are meaningless in a grid of a different width.
func TestEveryViewportResolvesToABucketWithTheRightColumnCount(t *testing.T) {
	for width := minSampleViewport; width <= maxPrebakeViewport; width++ {
		wantCols, _ := layoutStateForViewport(width)
		if got := bucketFor(width); got.Cols != wantCols {
			t.Fatalf("at %dpx the page uses bucket %s with %d columns, client packs %d",
				width, got.Name, got.Cols, wantCols)
		}
	}
}

// A bucket stores one height per plugin, measured at its sample width, so every
// viewport inside it is served a height measured at a different width. Keeping
// each bucket narrow in relative terms keeps that error small: a plugin's
// height tracks its column width, so a bucket spanning 37% of width (bp2 used
// to span 521..715) served heights that were hundreds of pixels off at the
// edges, and the page visibly settled on hydration.
func TestBucketsAreNarrowEnoughForOneSampleToRepresentThem(t *testing.T) {
	for i, b := range viewportBuckets {
		lo := b.MinWidth
		if lo < minSampleViewport {
			lo = minSampleViewport
		}
		hi := maxPrebakeViewport
		if i+1 < len(viewportBuckets) {
			hi = viewportBuckets[i+1].MinWidth - 1
		}
		if hi <= lo {
			continue
		}
		// past the width cap every viewport lays out identically
		if _, mw := layoutStateForViewport(lo); mw >= MaxMosaicWidth {
			continue
		}
		if ratio := float64(hi) / float64(lo); ratio > maxBucketWidthRatio+0.001 {
			t.Errorf("bucket %s spans %d..%d (ratio %.3f), wider than %.2f",
				b.Name, lo, hi, ratio, maxBucketWidthRatio)
		}
		if b.SampleWidth < lo || b.SampleWidth > hi {
			t.Errorf("bucket %s samples at %d, outside its own range %d..%d",
				b.Name, b.SampleWidth, lo, hi)
		}
	}
}

func TestExtractLayoutUsesViewportAwareWidthHint(t *testing.T) {
	html := `<section class="projects-section plugin" data-w="3" data-pb-h-1="111" data-pb-h-3="333"></section>`

	// Each hint is a measurement, so it is reserved verbatim — except where the
	// floor on plugin height lifts it (111 -> minPluginHeightCap).
	narrow := ExtractLayout(html, 0, ViewportBucket{Cols: 1, MosaicWidth: 320}, nil)
	if narrow.Width != 1 || narrow.Height != minPluginHeightCap {
		t.Fatalf("narrow layout = %+v, want width 1 height %d", narrow, minPluginHeightCap)
	}

	wide := ExtractLayout(html, 0, ViewportBucket{Cols: 4, MosaicWidth: 1240}, nil)
	if wide.Width != 3 || wide.Height != 333 {
		t.Fatalf("wide layout = %+v, want width 3 height 333", wide)
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

// Regression: expandHorizontally sorts placements in place, so Prebake must
// re-index by original plugin order — otherwise each plugin gets another
// plugin's height/position (the profile-overflow scramble bug).
func TestPrebakePreservesPluginMapping(t *testing.T) {
	htmls := []template.HTML{
		`<section class="webring-section plugin" data-w="2"></section>`,
		`<section class="profile-section plugin" data-w="2"><div class="plugin__inner"><div class="profile-bio">` +
			`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` +
			`</div></div></section>`,
		`<section class="health-section plugin" data-w="1"></section>`,
		`<section class="tech-section plugin" data-w="1"></section>`,
	}
	out := Prebake(htmls, nil)
	if len(out) != len(htmls) {
		t.Fatalf("Prebake returned %d, want %d", len(out), len(htmls))
	}
	// each emitted section must carry a plausible height for ITS OWN content;
	// the profile (long bio) must be the tallest at bp8.
	heightOf := func(section string) int {
		m := regexpFind(section, `--pb-bp8-h:(\d+)px`)
		if m == "" {
			return 0
		}
		n := 0
		for _, c := range m {
			n = n*10 + int(c-'0')
		}
		return n
	}
	profileH := heightOf(string(out[1]))
	for i, s := range out {
		if i == 1 {
			continue
		}
		if heightOf(string(s)) > profileH {
			t.Fatalf("plugin %d height %d exceeds profile %d — mapping likely scrambled",
				i, heightOf(string(s)), profileH)
		}
	}
	if profileH < 200 {
		t.Fatalf("profile height %d too small — bio not reserved", profileH)
	}
}

// A measured height came out of a real browser laying out this exact markup at
// this exact column width — the same number the client will measure on
// hydration. Padding it makes the prebaked box taller than the box the client
// settles on, and the shortest-column packer turns that difference into a
// different column, which is the violent first-paint reshuffle.
func TestMeasuredHeightIsReservedVerbatim(t *testing.T) {
	html := `<section class="tech-section plugin" data-w="1"></section>`
	bucket := ViewportBucket{Name: "bp0", Cols: 3, MosaicWidth: 900}
	store := fakeLookup{"tech|bp0|1": 1234}
	if got := ExtractLayout(html, 0, bucket, store).Height; got != 1234 {
		t.Fatalf("measured height reserved as %d, want exactly 1234", got)
	}
}

// The profile used to be barred from the store on the theory that measurement
// underestimates its wrapped bio. It does not: the measure worker clones the
// section at the target column width, which is exactly what the client packer
// does. Refusing the measurement over-reserved the card by ~60%, and that one
// bad height was enough to scatter most of the page into other columns.
func TestProfileUsesMeasuredHeight(t *testing.T) {
	html := `<section class="profile-section plugin" data-w="2"><div class="plugin__inner">` +
		`<div class="profile-bio">short bio</div></div></section>`
	bucket := ViewportBucket{Name: "bp8", Cols: 6, MosaicWidth: 1944}
	store := fakeLookup{"profile|bp8|2": 235}
	if got := ExtractLayout(html, 0, bucket, store).Height; got != 235 {
		t.Fatalf("profile height %d, want the measured 235", got)
	}
}

// End-to-end shape of the bug: with every height measured, the prebaked
// placement must be the placement the client's packer produces from those same
// heights — same column, same row, for every plugin. Any inflation the prebake
// applies on top of a measurement shows up here as a mismatch.
func TestPrebakedPlacementMatchesClientPackFromSameHeights(t *testing.T) {
	type plugin struct {
		name string
		span int
		h    int
	}
	// Heights a headless Chrome measured for the live page at a 2000px
	// viewport; the client settled on exactly these. The meme is left out: its
	// height comes from the served image by design, so it never matches a store.
	page := []plugin{
		{"webring", 2, 122}, {"profile", 2, 235}, {"social", 1, 223}, {"tech", 1, 360},
		{"health", 2, 131}, {"bike", 2, 749}, {"music", 2, 564},
		{"visitors", 1, 468}, {"code", 2, 1482}, {"neofetch", 2, 497}, {"places", 2, 398},
		{"steam", 2, 740}, {"services", 2, 1085}, {"info", 2, 224}, {"beatleader", 2, 733},
		{"photos", 2, 557},
	}
	// the bucket a 2000px viewport resolves to, which is where the capture below
	// was taken
	bucket := bucketFor(2000)
	bucketName := bucket.Name
	// the mosaic is capped at 1680px: five 300px columns
	if bucket.Cols != 5 {
		t.Fatalf("2000px resolves to %s with %d columns, expected five", bucketName, bucket.Cols)
	}

	store := fakeLookup{}
	htmls := make([]template.HTML, len(page))
	want := make([]PluginLayout, len(page))
	for i, p := range page {
		store[p.name+"|"+bucketName+"|"+strconv.Itoa(p.span)] = p.h
		htmls[i] = template.HTML(
			`<section class="` + p.name + `-section plugin" data-w="` + strconv.Itoa(p.span) + `"></section>`)
		want[i] = PluginLayout{Name: p.name, Width: p.span, Height: p.h, Order: i}
	}

	// What the client does: pack the measured heights.
	expected := map[string]Placement{}
	for _, p := range packForCols(want, bucket.Cols) {
		expected[p.Name] = p
	}

	// What the server reserves.
	for i, h := range Prebake(htmls, store) {
		name := page[i].name
		if name == "meme" {
			continue // random image per request, reserved from content by design
		}
		e := expected[name]
		col := regexpFind(string(h), `--pb-`+bucketName+`-col:(\d+)`)
		row := regexpFind(string(h), `--pb-`+bucketName+`-row:(\d+)`)
		if wantCol := strconv.Itoa(e.Col + 1); col != wantCol {
			t.Errorf("%s: prebaked column %s, client packs it at %s", name, col, wantCol)
		}
		if wantRow := strconv.Itoa(e.Row + 1); row != wantRow {
			t.Errorf("%s: prebaked row %s, client packs it at %s", name, row, wantRow)
		}
	}
}

func regexpFind(s, pat string) string {
	re := reMustCompile(pat)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// A narrow card fills the hole a wide card leaves above the shorter column
// (the meme next to coding stats). Shortest-column masonry put it at the bottom.
func TestPackFillsHolesLeftByWideCards(t *testing.T) {
	in := []PluginLayout{
		{Name: "short", Width: 1, Height: 100, Order: 0},
		{Name: "tall", Width: 1, Height: 500, Order: 1},
		{Name: "wide", Width: 2, Height: 100, Order: 2},
		{Name: "narrow", Width: 1, Height: 100, Order: 3},
	}
	got := map[string]Placement{}
	for _, p := range packForCols(in, 2) {
		got[p.Name] = p
	}
	if got["wide"].Row != 500+Gap {
		t.Fatalf("wide card at row %d, want under the tall one at %d", got["wide"].Row, 500+Gap)
	}
	if n := got["narrow"]; n.Col != 0 || n.Row != 100+Gap {
		t.Fatalf("narrow card at col %d row %d, want the hole at col 0 row %d", n.Col, n.Row, 100+Gap)
	}
}
