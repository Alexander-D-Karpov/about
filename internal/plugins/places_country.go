package plugins

import (
	"encoding/json"
	"log"
	"math"
	"sync"
)

// CountryShapesJSON is the world countries GeoJSON (static/libs/geo), set by
// main from the embedded static files. It lets a place without a "country"
// field be filed under the country its coordinates fall in.
var CountryShapesJSON []byte

type countryShape struct {
	name                   string
	polys                  [][][][2]float64 // polygon -> rings -> [lng, lat]
	minX, minY, maxX, maxY float64
}

var (
	countryShapesOnce sync.Once
	countryShapes     []countryShape
)

func loadCountryShapes() []countryShape {
	countryShapesOnce.Do(func() {
		if len(CountryShapesJSON) == 0 {
			return
		}
		var fc struct {
			Features []struct {
				Properties struct {
					Name string `json:"name"`
				} `json:"properties"`
				Geometry struct {
					Type        string          `json:"type"`
					Coordinates json.RawMessage `json:"coordinates"`
				} `json:"geometry"`
			} `json:"features"`
		}
		if err := json.Unmarshal(CountryShapesJSON, &fc); err != nil {
			log.Printf("[Places] country shapes: %v", err)
			return
		}
		for _, f := range fc.Features {
			var polys [][][][2]float64
			switch f.Geometry.Type {
			case "Polygon":
				var p [][][2]float64
				if json.Unmarshal(f.Geometry.Coordinates, &p) == nil {
					polys = append(polys, p)
				}
			case "MultiPolygon":
				_ = json.Unmarshal(f.Geometry.Coordinates, &polys)
			}
			if len(polys) == 0 {
				continue
			}
			s := countryShape{name: f.Properties.Name, polys: polys,
				minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
			for _, p := range polys {
				for _, pt := range p[0] {
					s.minX, s.maxX = math.Min(s.minX, pt[0]), math.Max(s.maxX, pt[0])
					s.minY, s.maxY = math.Min(s.minY, pt[1]), math.Max(s.maxY, pt[1])
				}
			}
			countryShapes = append(countryShapes, s)
		}
	})
	return countryShapes
}

func ringContains(ring [][2]float64, x, y float64) bool {
	in := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		xi, yi, xj, yj := ring[i][0], ring[i][1], ring[j][0], ring[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

func segDist(px, py float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l := dx*dx + dy*dy
	t := 0.0
	if l > 0 {
		t = math.Max(0, math.Min(1, ((px-a[0])*dx+(py-a[1])*dy)/l))
	}
	return math.Hypot(a[0]+t*dx-px, a[1]+t*dy-py)
}

// countryAt names the country at lat/lng. The shapes are simplified, so the
// test is fuzzy on both sides of an outline: a coastal point (Dubai, Narita)
// can sit just off its own polygon, and a point right on one country's
// outline can fall inside a neighbour's coarse polygon (Sochi lies inside
// the simplified Georgia but 0.003° from Russia's coast). Each nearby
// country scores its distance to the point - a containing polygon counts as
// a flat 0.01° - and
// the lowest score within 1.5° wins.
func countryAt(lat, lng float64) string {
	const containedScore, maxDist = 0.01, 1.5
	best, bestScore := "", maxDist
	for _, s := range loadCountryShapes() {
		if lng < s.minX-maxDist || lng > s.maxX+maxDist || lat < s.minY-maxDist || lat > s.maxY+maxDist {
			continue
		}
		score := math.Inf(1)
		for _, p := range s.polys {
			if len(p) == 0 {
				continue
			}
			if ringContains(p[0], lng, lat) {
				inHole := false
				for _, hole := range p[1:] {
					if ringContains(hole, lng, lat) {
						inHole = true
						break
					}
				}
				if !inHole {
					score = math.Min(score, containedScore)
					continue
				}
			}
			r := p[0]
			for i := 0; i+1 < len(r); i++ {
				score = math.Min(score, segDist(lng, lat, r[i], r[i+1]))
			}
		}
		if score < bestScore {
			best, bestScore = s.name, score
		}
	}
	return best
}
