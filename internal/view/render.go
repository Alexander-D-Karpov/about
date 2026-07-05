package view

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"strings"
)

func maxF(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	if m == 0 {
		m = 1
	}
	return m
}

func maxI(v []int) int {
	m := 1
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}

func barsSpark(hs []int) template.HTML {
	m := maxI(hs)
	var b strings.Builder
	for _, h := range hs {
		fmt.Fprintf(&b, `<span style="width:3px;height:%dpx;background:rgba(10,14,20,.55);border-radius:1px"></span>`, int(math.Round(float64(h)/float64(m)*22)))
	}
	return template.HTML(`<span style="display:inline-flex;align-items:flex-end;gap:1.5px;height:22px">` + b.String() + `</span>`)
}

func spark(kind string, data []float64, color string) template.HTML {
	switch kind {
	case "ecg":
		beat := "M0 11 H18 l4 -7 l4 14 l4 -9 l2 2 H58 l4 -7 l4 14 l4 -9 l2 2 H120"
		return template.HTML(`<span style="display:block;margin-top:7px"><svg width="100%" height="20" viewBox="0 0 120 22" preserveAspectRatio="none" fill="none" stroke="` + color + `" stroke-width="1.6" stroke-linejoin="round" style="display:block;overflow:hidden"><g style="animation:scScroll 1.6s linear infinite"><path d="` + beat + `"/><path transform="translate(120,0)" d="` + beat + `"/></g></svg></span>`)
	case "tiles":
		m := maxF(data)
		var t strings.Builder
		for _, v := range data {
			fmt.Fprintf(&t, `<span style="flex:1;aspect-ratio:1;border-radius:3px;background:%s;opacity:%.2f"></span>`, color, 0.16+(v/m)*0.84)
		}
		return template.HTML(`<span style="display:flex;gap:4px;margin-top:10px;max-width:150px">` + t.String() + `</span>`)
	case "battery":
		return batterySpark(data, color)
	default:
		m := maxF(data)
		var t strings.Builder
		for i, v := range data {
			op := "0.42"
			if i == len(data)-1 {
				op = "1"
			}
			fmt.Fprintf(&t, `<span style="flex:1;min-width:5px;height:%dpx;background:%s;opacity:%s;border-radius:2px"></span>`, int(math.Round(v/m*26)), color, op)
		}
		return template.HTML(`<span style="display:flex;align-items:flex-end;gap:3px;height:28px;margin-top:10px">` + t.String() + `</span>`)
	}
}

func batterySpark(hs []float64, color string) template.HTML {
	const W, H, cw = 168.0, 40.0, 140.0
	y := func(v float64) float64 { return 4 + (1-v/100)*(H-8) }
	n := len(hs)
	pts := make([][2]float64, n)
	for i, v := range hs {
		pts[i] = [2]float64{float64(i) / float64(n-1) * cw, y(v)}
	}
	var line, area strings.Builder
	for i, p := range pts {
		c := "L"
		if i == 0 {
			c = "M"
		}
		fmt.Fprintf(&line, "%s%.1f %.1f ", c, p[0], p[1])
	}
	fmt.Fprintf(&area, "M0 %.0f ", H-4)
	for _, p := range pts {
		fmt.Fprintf(&area, "L%.1f %.1f ", p[0], p[1])
	}
	fmt.Fprintf(&area, "L%.0f %.0f Z", cw, H-4)
	last := pts[n-1]
	svg := fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" style="display:block;width:100%%;height:40px;overflow:visible"><path d="%s" fill="%s" fill-opacity="0.13"/><path d="%s" fill="none" stroke="%s" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/><circle cx="%.1f" cy="%.1f" r="2.4" fill="%s"/></svg>`,
		W, H, area.String(), color, line.String(), color, last[0], last[1], color)
	return template.HTML(`<span style="display:block;margin-top:10px;color:var(--muted)">` + svg + `</span>`)
}

func diffBar(g int) template.HTML {
	if g < 0 {
		g = 0
	}
	if g > 5 {
		g = 5
	}
	var b strings.Builder
	for i := 0; i < 5; i++ {
		c := "#f0816a"
		if i < g {
			c = "#3fb950"
		}
		fmt.Fprintf(&b, `<span style="width:7px;height:7px;border-radius:1.5px;background:%s"></span>`, c)
	}
	return template.HTML(`<span style="display:inline-flex;gap:1.5px">` + b.String() + `</span>`)
}

var funcs = template.FuncMap{
	"icon":      Icon,
	"spark":     spark,
	"barsSpark": barsSpark,
	"diffBar":   diffBar,
	"bikeSpark": bikeSpark,
}

func bikeSpark(prof []float64, color string) template.HTML {
	if len(prof) < 2 {
		return ""
	}
	min, max := prof[0], prof[0]
	for _, v := range prof {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	if rng == 0 {
		rng = 1
	}
	const w, h = 220.0, 26.0
	pts := make([]string, len(prof))
	for i, v := range prof {
		x := float64(i) / float64(len(prof)-1) * w
		y := h - ((v-min)/rng)*(h-3) - 1.5
		pts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	line := "M" + strings.Join(pts, " L")
	fill := fmt.Sprintf("M0,%.0f L%s L%.0f,%.0f Z", h, strings.Join(pts, " L"), w, h)
	return template.HTML(fmt.Sprintf(`<svg class="ride-spark" viewBox="0 0 %.0f %.0f" preserveAspectRatio="none"><path d="%s" fill="%s" fill-opacity="0.13"/><path d="%s" fill="none" stroke="%s" stroke-width="1.4" stroke-linejoin="round"/></svg>`, w, h, fill, color, line, color))
}

var adminFuncs = template.FuncMap{
	"json": func(v interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			return "{}"
		}
		return string(b)
	},
	"jsonPretty": func(v interface{}) string {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "{}"
		}
		return string(b)
	},
}

var tpl = template.Must(template.New("index.html").Funcs(funcs).ParseFiles("templates/index.html"))
var adminTpl = template.Must(template.New("admin.html").Funcs(adminFuncs).ParseFiles("templates/admin.html"))

func RenderPage(vm PageVM) (template.HTML, error) {
	var b bytes.Buffer
	if err := tpl.ExecuteTemplate(&b, "page", vm); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func RenderSection(name string, vm any) (string, error) {
	var b bytes.Buffer
	if err := tpl.ExecuteTemplate(&b, name, vm); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderAdmin(vm AdminVM) (template.HTML, error) {
	var b bytes.Buffer
	if err := adminTpl.ExecuteTemplate(&b, "admin", vm); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}
