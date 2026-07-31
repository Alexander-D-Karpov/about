package layout

import (
	"fmt"
	"html/template"
	"math"
	"sort"
	"strconv"
	"strings"
)

type PluginLayout struct {
	Name   string
	Width  int
	Height int
	Order  int
}

type Placement struct {
	Name    string
	Order   int // original plugin index; preserved across expandHorizontally's sort
	Col     int
	Row     int
	ColSpan int
	RowSpan int
	Height  int
}

type ViewportBucket struct {
	Name        string
	MinWidth    int
	SampleWidth int
	Cols        int
	MosaicWidth int
}

const (
	RowHeight          = 1
	Gap                = 12
	minSampleViewport  = 320
	maxPrebakeViewport = 3840
	maxPluginHeightCap = 2600
	minPluginHeightCap = 120
	heuristicBiasFrac  = 0.06
	measuredBiasFrac   = 0.02
	biasPad            = 8
)

var viewportBuckets = buildViewportBuckets()

func normalizePluginName(name string) string {
	switch strings.ToLower(name) {
	case "techstack":
		return "tech"
	case "links":
		return "social"
	default:
		return strings.ToLower(name)
	}
}

func extractPluginName(html string) string {
	lower := strings.ToLower(html)
	idx := strings.Index(lower, "-section")
	if idx < 0 {
		return ""
	}

	start := idx
	for start > 0 {
		c := lower[start-1]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			start--
		} else {
			break
		}
	}

	return normalizePluginName(lower[start:idx])
}

// profileBioChars returns the visible character count of the profile bio text
// (the content of the .profile-bio element), used to reserve enough height for
// the wrapped bio without relying on measurement.
func profileBioChars(html string) int {
	lower := strings.ToLower(html)
	idx := strings.Index(lower, "profile-bio")
	if idx < 0 {
		return 0
	}
	gt := strings.IndexByte(html[idx:], '>')
	if gt < 0 {
		return 0
	}
	start := idx + gt + 1
	end := strings.Index(lower[start:], "</div>")
	if end < 0 {
		end = len(html) - start
	}
	text := html[start : start+end]

	// strip any nested tags and collapse entities to a single char each.
	var b strings.Builder
	depth := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '<':
			depth++
		case c == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteByte(c)
		}
	}
	n := 0
	inEntity := false
	for _, r := range strings.TrimSpace(b.String()) {
		if r == '&' {
			inEntity = true
			n++
			continue
		}
		if inEntity {
			if r == ';' {
				inEntity = false
			}
			continue
		}
		n++
	}
	return n
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func extractIntAttr(html, attr string) (int, bool) {
	lower := strings.ToLower(html)
	key := strings.ToLower(attr) + `="`
	idx := strings.Index(lower, key)
	if idx < 0 {
		return 0, false
	}

	start := idx + len(key)
	end := start
	for end < len(html) && html[end] >= '0' && html[end] <= '9' {
		end++
	}
	if end == start {
		return 0, false
	}

	v, err := strconv.Atoi(html[start:end])
	if err != nil {
		return 0, false
	}
	return v, true
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func countToken(html, token string) int {
	return strings.Count(strings.ToLower(html), strings.ToLower(token))
}

func roundInt(v float64) int {
	return int(math.Round(v))
}

func floorInt(v float64) int {
	return int(math.Floor(v))
}

func biasUp(raw int, measured bool) int {
	frac := heuristicBiasFrac
	if measured {
		frac = measuredBiasFrac
	}
	return int(math.Ceil(float64(raw)*(1+frac))) + biasPad
}

func containerPaddingForViewport(viewportWidth int) float64 {
	if viewportWidth <= 780 {
		return 16
	}

	padding := float64(viewportWidth) * 0.03
	if padding < 12 {
		return 12
	}
	if padding > 28 {
		return 28
	}
	return padding
}

func colMinForViewport(viewportWidth int) int {
	switch {
	case viewportWidth <= 520:
		return 160
	case viewportWidth <= 780:
		return 220
	case viewportWidth <= 900:
		return 240
	case viewportWidth <= 1200:
		return 260
	default:
		return 300
	}
}

func layoutStateForViewport(viewportWidth int) (cols, mosaicWidth int) {
	padding := containerPaddingForViewport(viewportWidth)
	mosaicWidth = max(0, floorInt(float64(viewportWidth)-padding*2))
	cols = max(1, (mosaicWidth+Gap)/(colMinForViewport(viewportWidth)+Gap))
	return cols, mosaicWidth
}

func buildViewportBuckets() []ViewportBucket {
	type state struct {
		Cols   int
		MinCol int
	}

	var minWidths []int
	lastState := state{}
	initialized := false

	for viewportWidth := 0; viewportWidth <= maxPrebakeViewport; viewportWidth++ {
		cols, _ := layoutStateForViewport(viewportWidth)
		current := state{
			Cols:   cols,
			MinCol: colMinForViewport(viewportWidth),
		}

		if !initialized || current != lastState {
			minWidths = append(minWidths, viewportWidth)
			lastState = current
			initialized = true
		}
	}

	buckets := make([]ViewportBucket, 0, len(minWidths))
	for i, minWidth := range minWidths {
		rangeStart := max(minWidth, minSampleViewport)
		rangeEnd := maxPrebakeViewport
		if i+1 < len(minWidths) {
			rangeEnd = minWidths[i+1] - 1
		}
		if rangeEnd < rangeStart {
			rangeEnd = rangeStart
		}

		sampleWidth := rangeStart + (rangeEnd-rangeStart)/2
		cols, mosaicWidth := layoutStateForViewport(sampleWidth)

		buckets = append(buckets, ViewportBucket{
			Name:        fmt.Sprintf("bp%d", i),
			MinWidth:    minWidth,
			SampleWidth: sampleWidth,
			Cols:        cols,
			MosaicWidth: mosaicWidth,
		})
	}

	return buckets
}

func (b ViewportBucket) pluginWidth(span int) int {
	span = clamp(span, 1, max(1, b.Cols))
	if b.Cols <= 0 {
		return 0
	}

	colWidth := float64(b.MosaicWidth-Gap*(b.Cols-1)) / float64(b.Cols)
	return max(0, roundInt(colWidth*float64(span))+Gap*(span-1))
}

func contentWidth(pluginWidth int) int {
	return max(pluginWidth-28, 120)
}

func itemsPerRowByPixelWidth(usableWidth, minItemWidth, maxCols int) int {
	if minItemWidth <= 0 {
		return 1
	}

	cols := max(1, usableWidth/minItemWidth)
	if maxCols > 0 && cols > maxCols {
		cols = maxCols
	}
	return cols
}

func estimateHeightFromHTML(name string, span, pluginWidth int, html string) int {
	lower := strings.ToLower(html)
	usableWidth := contentWidth(pluginWidth)
	h := 240

	switch name {
	case "profile":
		// The profile bio wraps taller as the column narrows, the packer can
		// place the card at span 1 (narrowest) regardless of data-w, and text
		// height depends on the viewer's fonts — so measurement underestimates
		// it. Reserve deterministically and conservatively from the bio length
		// at the given (span-1) width. usableWidth here is the span-1 width
		// (ExtractLayout passes span 1 for the profile).
		const (
			profileHeaderH = 168 // name + role lines + avatar block + paddings
			profileCharPx  = 11  // deliberately wide: over-counts lines vs word-wrap
			profileLineH   = 24
		)
		bioChars := profileBioChars(html)
		if bioChars > 0 {
			cpl := usableWidth / profileCharPx
			if cpl < 1 {
				cpl = 1
			}
			lines := ceilDiv(bioChars, cpl)
			h = profileHeaderH + lines*profileLineH
		} else {
			h = profileHeaderH
		}

	case "social":
		icons := countToken(lower, "social-link")
		pages := countToken(lower, "social-page-link")
		iconRows := ceilDiv(max(icons, 1), itemsPerRowByPixelWidth(usableWidth, 48, 8))
		pageRows := 0
		if pages > 0 {
			pageRows = ceilDiv(pages, itemsPerRowByPixelWidth(usableWidth, 170, 3))
		}
		h = 100 + iconRows*56 + pageRows*42

	case "tech":
		items := countToken(lower, "tech-item")
		rows := ceilDiv(max(items, 1), itemsPerRowByPixelWidth(usableWidth, 92, 6))
		h = 100 + rows*60

	case "projects":
		items := countToken(lower, "project-card")
		rows := ceilDiv(max(items, 1), itemsPerRowByPixelWidth(usableWidth, 260, 3))
		h = 96 + rows*286

	case "photos":
		items := countToken(lower, "photos-card")
		rows := ceilDiv(max(items, 1), itemsPerRowByPixelWidth(usableWidth, 210, 4))
		h = 110 + rows*260

	case "bike":
		rides := countToken(lower, "bike-ride-item")
		mapH := 340
		statsH := 120
		listRows := min(max(rides, 1), 6)
		h = mapH + statsH + listRows*88 + 48

	case "places":
		mapH := 320
		h = mapH + 80

	case "health":
		cards := countToken(lower, `data-metric=`)
		if cards == 0 {
			cards = countToken(lower, `class="health-card"`)
		}
		cols := itemsPerRowByPixelWidth(usableWidth, 110, 4)
		rows := ceilDiv(max(cards, 1), cols)
		h = 88 + rows*84

	case "info":
		items := countToken(lower, "info-item")
		akaTags := countToken(lower, "aka-tag")
		cols := itemsPerRowByPixelWidth(usableWidth, 260, 2)
		rows := ceilDiv(max(items, 1), cols)
		akaH := 0
		if akaTags > 0 {
			akaRows := ceilDiv(akaTags, itemsPerRowByPixelWidth(usableWidth, 90, 6))
			akaH = 40 + akaRows*28
		}
		h = 100 + rows*46 + akaH + 40

	case "visitors":
		stats := countToken(lower, "visitor-stat")
		rows := ceilDiv(max(stats, 1), 3)
		h = 90 + rows*70
		if strings.Contains(lower, "visitors-chart") {
			h += 60
		}
		if strings.Contains(lower, "visitors-map") {
			h += 296
		}

	case "services":
		items := countToken(lower, "service-item")
		rows := ceilDiv(max(items, 1), itemsPerRowByPixelWidth(usableWidth, 290, 3))
		h = 132 + rows*158 + 116

	case "personal":
		items := countToken(lower, "personal-item")
		hasImage := strings.Contains(lower, "personal-image")
		cols := itemsPerRowByPixelWidth(usableWidth, 320, 2)
		rows := ceilDiv(max(items, 1), cols)
		perItem := 240
		if hasImage {
			perItem = 380
		}
		h = 110 + rows*perItem

	case "meme":
		captionH := 0
		if countToken(lower, "meme-caption") > 0 {
			captionH = 44
		}
		if iw, okw := extractIntAttr(html, "data-pb-imgw"); okw && iw > 0 {
			// Deterministic: the served image renders at usableWidth, scaled by
			// its real aspect ratio and clamped to the CSS max-height (800px).
			// This matches the ACTUAL meme being served, so a random meme can
			// never overflow its reserved track.
			ih, _ := extractIntAttr(html, "data-pb-imgh")
			imgH := 260
			if ih > 0 {
				imgH = roundInt(float64(usableWidth) * float64(ih) / float64(iw))
			}
			if imgH > 800 {
				imgH = 800
			}
			if imgH < 120 {
				imgH = 120
			}
			h = imgH + captionH + 96
		} else if strings.Contains(lower, "<img") {
			// Unknown/remote image: reserve generously (portrait-ish) rather than
			// the old tight guess, so an unmeasured tall meme still can't overflow.
			imgH := min(int(float64(usableWidth)*1.4), 800)
			if imgH < 260 {
				imgH = 260
			}
			h = imgH + captionH + 96
		} else {
			lines := countToken(lower, "<p")
			if lines < 1 {
				lines = 1
			}
			h = 200 + lines*28
		}

	case "music":
		recent := countToken(lower, "music-recent__item")
		nowBlock := 176
		visibleRecent := min(recent, 5)
		statsH := 0
		if strings.Contains(lower, "music-stats") {
			statsH = 48
		}
		h = 120 + nowBlock + visibleRecent*62 + statsH

	case "steam":
		games := countToken(lower, "game-item")
		rows := ceilDiv(max(games, 1), itemsPerRowByPixelWidth(usableWidth, 220, 3))
		extra := 120
		if strings.Contains(lower, "current-game") {
			coverH := pluginWidth / 2
			if coverH < 120 {
				coverH = 120
			}
			if coverH > 240 {
				coverH = 240
			}
			extra += coverH + 90
		} else if strings.Contains(lower, "player-status") {
			extra += 80
		}
		h = 130 + extra + rows*96

	case "beatleader":
		maps := countToken(lower, "map-item")
		statsCols := itemsPerRowByPixelWidth(usableWidth, 140, 4)
		statsRows := ceilDiv(4, max(statsCols, 1))
		mapsCols := itemsPerRowByPixelWidth(usableWidth, 280, 2)
		mapsRows := ceilDiv(max(maps, 1), max(mapsCols, 1))
		h = 110 + statsRows*92 + 40 + mapsRows*92 + 60

	case "code":
		wakaItems := countToken(lower, "waka-item")
		summaryH := 120
		statsH := 140
		heatmapH := 220
		togglesH := 6 * 48
		langsH := 320
		openH := min(wakaItems, 14) * 40
		h = 140 + summaryH + statsH + heatmapH + togglesH + langsH + openH

	case "neofetch":
		h = 440

	case "webring":
		h = 116
	}

	h += 16
	if h > maxPluginHeightCap {
		h = maxPluginHeightCap
	}
	if h < minPluginHeightCap {
		h = minPluginHeightCap
	}
	return h
}

// HeightLookup resolves a previously-measured plugin height by plugin name,
// viewport bucket, and column span.
type HeightLookup interface {
	Get(plugin, bucket string, span int) (int, bool)
}

// ViewportBuckets returns the precomputed responsive buckets.
func ViewportBuckets() []ViewportBucket {
	out := make([]ViewportBucket, len(viewportBuckets))
	copy(out, viewportBuckets)
	return out
}

// PluginWidth is the exported column-pixel width of a span in this bucket.
func (b ViewportBucket) PluginWidth(span int) int { return b.pluginWidth(span) }

func ExtractLayout(html string, order int, bucket ViewportBucket, store HeightLookup) PluginLayout {
	name := extractPluginName(html)

	baseWidth := 1
	if v, ok := extractIntAttr(html, "data-w"); ok && v >= 1 {
		baseWidth = v
	}

	w := clamp(baseWidth, 1, max(1, bucket.Cols))
	pluginWidth := bucket.pluginWidth(w)

	height := 0
	measured := false
	// meme (random per request) and profile (bio height depends on span/width/
	// fonts, and the packer may down-span it) both underestimate under
	// measurement, so they never trust the store — their heights are reserved
	// deterministically from content in estimateHeightFromHTML instead.
	storeTrusted := name != "meme" && name != "profile"
	if store != nil && storeTrusted {
		if h, ok := store.Get(name, bucket.Name, w); ok && h > 0 {
			height = h
			measured = true
		}
	}
	if !measured {
		if v, ok := extractIntAttr(html, fmt.Sprintf("data-pb-h-%d", w)); ok && storeTrusted && v > 0 {
			height = v
			measured = true
		} else if v, ok := extractIntAttr(html, "data-pb-h"); ok && storeTrusted && v > 0 {
			height = v
			measured = true
		} else {
			height = estimateHeightFromHTML(name, w, pluginWidth, html)
		}
	}

	height = biasUp(height, measured)

	if height > maxPluginHeightCap {
		height = maxPluginHeightCap
	}
	if height < minPluginHeightCap {
		height = minPluginHeightCap
	}

	return PluginLayout{
		Name:   name,
		Width:  w,
		Height: height,
		Order:  order,
	}
}

func packForCols(plugins []PluginLayout, cols int) []Placement {
	placements := make([]Placement, 0, len(plugins))
	colHeights := make([]int, cols)

	for _, p := range plugins {
		w := p.Width
		if w > cols {
			w = cols
		}

		bestCol := 0
		bestHeight := math.MaxInt32

		for c := 0; c <= cols-w; c++ {
			maxH := 0
			for i := c; i < c+w; i++ {
				if colHeights[i] > maxH {
					maxH = colHeights[i]
				}
			}
			if maxH < bestHeight {
				bestHeight = maxH
				bestCol = c
			}
		}

		startRow := int(math.Ceil(float64(bestHeight) / float64(RowHeight)))
		rowSpan := int(math.Ceil(float64(p.Height+Gap) / float64(RowHeight)))
		newHeight := bestHeight + p.Height + Gap

		for i := bestCol; i < bestCol+w; i++ {
			colHeights[i] = newHeight
		}

		placements = append(placements, Placement{
			Name:    p.Name,
			Order:   p.Order,
			Col:     bestCol,
			Row:     startRow,
			ColSpan: w,
			RowSpan: rowSpan,
			Height:  p.Height,
		})
	}

	expandHorizontally(placements, cols)
	return placements
}

func expandHorizontally(placements []Placement, cols int) {
	sort.Slice(placements, func(i, j int) bool {
		if placements[i].Row != placements[j].Row {
			return placements[i].Row < placements[j].Row
		}
		return placements[i].Col < placements[j].Col
	})

	for i := range placements {
		p := &placements[i]

		for p.Col+p.ColSpan < cols {
			nextCol := p.Col + p.ColSpan
			canExpand := true

			for j := range placements {
				if i == j {
					continue
				}
				other := &placements[j]

				overlapsNextCol := other.Col == nextCol || (other.Col < nextCol && other.Col+other.ColSpan > nextCol)
				if !overlapsNextCol {
					continue
				}

				pEnd := p.Row + p.RowSpan
				oEnd := other.Row + other.RowSpan
				if !(p.Row >= oEnd || pEnd <= other.Row) {
					canExpand = false
					break
				}
			}

			if canExpand {
				p.ColSpan++
			} else {
				break
			}
		}
	}
}

type prebakeItem struct {
	HTML   template.HTML
	Layout PluginLayout
}

// fixedEdgePackOrder returns plugin indices in the order the client packs them
// (enforceFixedEdgeOrder in windowManager.js): webring first, profile second,
// then everything else in original order. Matching this keeps the prebaked
// layout aligned with masonry so the profile stays first and nothing jumps.
func fixedEdgePackOrder(pluginHTMLs []template.HTML) []int {
	var webring, profile, rest []int
	for i, h := range pluginHTMLs {
		s := string(h)
		switch {
		case strings.Contains(s, "webring-section"):
			webring = append(webring, i)
		case strings.Contains(s, "profile-section"):
			profile = append(profile, i)
		default:
			rest = append(rest, i)
		}
	}
	out := make([]int, 0, len(pluginHTMLs))
	out = append(out, webring...)
	out = append(out, profile...)
	out = append(out, rest...)
	return out
}

func Prebake(pluginHTMLs []template.HTML, store HeightLookup) []template.HTML {
	if len(pluginHTMLs) == 0 {
		return nil
	}

	// Pack in the same fixed-edge order the client's enforceFixedEdgeOrder uses
	// (webring, then profile, then the rest in original order) so the prebaked
	// positions match what masonry produces — the profile stays first and the
	// first paint doesn't jump on hydration.
	packSeq := fixedEdgePackOrder(pluginHTMLs)

	bucketPlacements := make(map[string][]Placement, len(viewportBuckets))
	for _, bucket := range viewportBuckets {
		layouts := make([]PluginLayout, len(pluginHTMLs))
		for i, h := range pluginHTMLs {
			layouts[i] = ExtractLayout(string(h), i, bucket, store)
		}
		packInput := make([]PluginLayout, len(pluginHTMLs))
		for pos, idx := range packSeq {
			packInput[pos] = layouts[idx]
		}
		// packForCols runs expandHorizontally, which sorts by (row,col); each
		// Placement carries its original plugin index in Order, so re-index by
		// Order to restore plugin-i alignment for the emit loop below.
		packed := packForCols(packInput, bucket.Cols)
		ordered := make([]Placement, len(pluginHTMLs))
		for _, p := range packed {
			if p.Order >= 0 && p.Order < len(ordered) {
				ordered[p.Order] = p
			}
		}
		bucketPlacements[bucket.Name] = ordered
	}

	result := make([]template.HTML, len(pluginHTMLs))
	for i, h := range pluginHTMLs {
		htmlStr := string(h)

		var attrs strings.Builder
		attrs.WriteString(` data-prebaked="1"`)

		var styleVars strings.Builder
		for _, bucket := range viewportBuckets {
			p := bucketPlacements[bucket.Name][i]
			styleVars.WriteString(fmt.Sprintf(
				"--pb-%s-col:%d;--pb-%s-cs:%d;--pb-%s-row:%d;--pb-%s-rs:%d;--pb-%s-h:%dpx;",
				bucket.Name, p.Col+1,
				bucket.Name, p.ColSpan,
				bucket.Name, p.Row+1,
				bucket.Name, p.RowSpan,
				bucket.Name, p.Height,
			))
		}

		injected := injectAttributes(htmlStr, attrs.String(), styleVars.String())
		result[i] = template.HTML(injected)
	}

	return result
}

func injectAttributes(html, extraAttrs, extraStyle string) string {
	sectionIdx := strings.Index(html, "<section")
	if sectionIdx < 0 {
		sectionIdx = strings.Index(html, "<div")
		if sectionIdx < 0 {
			return html
		}
	}

	tagEnd := strings.Index(html[sectionIdx:], ">")
	if tagEnd < 0 {
		return html
	}
	tagEnd += sectionIdx

	tag := html[sectionIdx:tagEnd]

	styleIdx := strings.Index(tag, `style="`)
	var newTag string

	if styleIdx >= 0 {
		insertAt := styleIdx + len(`style="`)
		newTag = tag[:insertAt] + extraStyle + tag[insertAt:]
	} else {
		newTag = tag + ` style="` + extraStyle + `"`
	}

	newTag += extraAttrs

	return html[:sectionIdx] + newTag + html[tagEnd:]
}

func GenerateResponsiveCSS() template.CSS {
	var css strings.Builder

	if len(viewportBuckets) == 0 {
		return ""
	}

	writeBucketCSS := func(bucket ViewportBucket) {
		css.WriteString(fmt.Sprintf(`.mosaic-prebaked{display:grid;grid-template-columns:repeat(%d,minmax(0,1fr));grid-auto-rows:1px;row-gap:0;column-gap:12px;align-items:start;}`, bucket.Cols))
		css.WriteString(`.mosaic-prebaked>.plugin{`)
		css.WriteString(`align-self:start;`)
		css.WriteString(`grid-column:var(--pb-` + bucket.Name + `-col)/span var(--pb-` + bucket.Name + `-cs);`)
		css.WriteString(`grid-row:var(--pb-` + bucket.Name + `-row)/span var(--pb-` + bucket.Name + `-rs);`)
		css.WriteString(`height:var(--pb-` + bucket.Name + `-h);`)
		css.WriteString(`min-height:var(--pb-` + bucket.Name + `-h);`)
		css.WriteString(`}`)
	}

	writeBucketCSS(viewportBuckets[0])
	for _, bucket := range viewportBuckets[1:] {
		css.WriteString(fmt.Sprintf(`@media(min-width:%dpx){`, bucket.MinWidth))
		writeBucketCSS(bucket)
		css.WriteString(`}`)
	}

	return template.CSS(css.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
