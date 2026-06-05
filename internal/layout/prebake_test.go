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

	narrow := ExtractLayout(html, 0, ViewportBucket{Cols: 1, MosaicWidth: 320})
	if narrow.Width != 1 || narrow.Height != 111 {
		t.Fatalf("narrow layout = %+v, want width 1 height 111", narrow)
	}

	wide := ExtractLayout(html, 0, ViewportBucket{Cols: 4, MosaicWidth: 1240})
	if wide.Width != 3 || wide.Height != 333 {
		t.Fatalf("wide layout = %+v, want width 3 height 333", wide)
	}
}

func TestPrebakeAndCSSExposeViewportHeightVars(t *testing.T) {
	plugins := []template.HTML{
		template.HTML(`<section class="profile-section plugin" data-w="2"></section>`),
	}

	prebaked := string(Prebake(plugins)[0])
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
