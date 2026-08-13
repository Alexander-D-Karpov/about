// Command bike is a small CLI to maintain the bike plugin's rides on a running
// about instance via the admin API.
//
// Its main job is reprocessing: re-running the server-side GPX parser over every
// ride that still has its uploaded GPX on disk, then saving the recomputed
// coordinates/distance/elevation/duration back. Use it after changing the GPX
// parser (e.g. the moving-time / average-speed logic) so existing rides pick up
// the new computation — including re-adding per-point timestamps to old rides
// that were stored without them.
//
// Usage:
//
//	bike list                 # list rides on the target (and whether each has a stored GPX)
//	bike reprocess            # reprocess every ride that has a stored GPX and save
//	bike reprocess -index 2   # reprocess only ride #2 (0-based)
//	bike reprocess -dry       # show what would change without saving
//
// Target + credentials come from flags or the environment (.env is loaded from
// the working dir): DEPLOY_URL / ADMIN_USER / ADMIN_PASS. Defaults target
// https://about.akarpov.ru with basic auth. Because the default is prod and this
// command mutates data, do a -dry run first and prefer pointing -url at a local
// or preview instance for the real push.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// pluginData mirrors the admin API's /api/plugins entry (only what we need).
type pluginData struct {
	Name     string                 `json:"name"`
	Enabled  bool                   `json:"enabled"`
	Order    int                    `json:"order"`
	Settings map[string]interface{} `json:"settings"`
}

type opts struct {
	url, user, pass string
	index           int
	dry             bool
}

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "list":
		cmdList(parse(os.Args[2:]))
	case "reprocess":
		cmdReprocess(parse(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `bike - maintain bike rides on a running about instance

  bike list         list rides on the target (name, date, distance, stored GPX?)
  bike reprocess    re-parse stored GPX for every ride and save the results

Flags:
  -url     target base URL (default $DEPLOY_URL or https://about.akarpov.ru)
  -user    admin user (default $ADMIN_USER)
  -pass    admin password (default $ADMIN_PASS)
  -index   (reprocess) only this ride index (0-based); default all
  -dry     (reprocess) show what would change without saving
`)
}

func parse(args []string) opts {
	o := opts{
		url:   envOr("DEPLOY_URL", "https://about.akarpov.ru"),
		user:  os.Getenv("ADMIN_USER"),
		pass:  os.Getenv("ADMIN_PASS"),
		index: -1,
	}
	for i := 0; i < len(args); i++ {
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch args[i] {
		case "-url":
			o.url = next()
		case "-user":
			o.user = next()
		case "-pass":
			o.pass = next()
		case "-index":
			n, err := strconv.Atoi(next())
			if err != nil || n < 0 {
				fmt.Fprintln(os.Stderr, "error: -index must be a non-negative integer")
				os.Exit(2)
			}
			o.index = n
		case "-dry":
			o.dry = true
		}
	}
	o.url = strings.TrimRight(o.url, "/")
	return o
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// fetchBike returns the bike plugin's current config from the target.
func fetchBike(o opts) pluginData {
	req, _ := http.NewRequest("GET", o.url+"/admin/api/plugins", nil)
	req.SetBasicAuth(o.user, o.pass)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	check(err)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		fatalf("GET plugins: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var plugins []pluginData
	check(json.NewDecoder(resp.Body).Decode(&plugins))
	for _, p := range plugins {
		if p.Name == "bike" {
			if p.Settings == nil {
				p.Settings = map[string]interface{}{}
			}
			return p
		}
	}
	fatalf("no bike plugin on %s", o.url)
	return pluginData{}
}

func rides(p pluginData) []map[string]interface{} {
	raw, _ := p.Settings["rides"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func str(m map[string]interface{}, k string) string {
	s, _ := m[k].(string)
	return s
}

func cmdList(o opts) {
	p := fetchBike(o)
	rs := rides(p)
	if len(rs) == 0 {
		fmt.Println("(no rides on", o.url+")")
		return
	}
	for i, r := range rs {
		gpx := "no GPX"
		if str(r, "gpx_file") != "" {
			gpx = "GPX ✓"
		}
		fmt.Printf("%2d. %-30s %-12s %6.1f km  %s\n",
			i, truncate(str(r, "name"), 30), str(r, "date"), num(r["distance_km"]), gpx)
	}
	fmt.Printf("\n%d ride(s) on %s\n", len(rs), o.url)
}

func cmdReprocess(o opts) {
	p := fetchBike(o)
	rs := rides(p)
	if len(rs) == 0 {
		fmt.Println("(no rides to reprocess on", o.url+")")
		return
	}

	var changed, skipped int
	for i, r := range rs {
		if o.index >= 0 && i != o.index {
			continue
		}
		name := str(r, "name")
		if name == "" {
			name = fmt.Sprintf("ride %d", i)
		}
		gpxFile := str(r, "gpx_file")
		if gpxFile == "" {
			fmt.Printf("  skip  %2d %-30s (no stored GPX)\n", i, truncate(name, 30))
			skipped++
			continue
		}

		newRide, err := reprocessOne(o, i, gpxFile)
		if err != nil {
			fmt.Printf("  FAIL  %2d %-30s %v\n", i, truncate(name, 30), err)
			skipped++
			continue
		}

		oldDist, oldElev, oldDur := num(r["distance_km"]), num(r["elevation_gain_m"]), num(r["duration_minutes"])
		// Overwrite only the parser-computed fields; leave name/date/hide_* etc.
		for _, k := range []string{"coordinates", "distance_km", "elevation_gain_m", "duration_minutes"} {
			if v, ok := newRide[k]; ok {
				r[k] = v
			}
		}
		fmt.Printf("  ok    %2d %-30s %.1f→%.1f km  ↑%.0f→%.0f m  %.0f→%.0f min\n",
			i, truncate(name, 30),
			oldDist, num(r["distance_km"]),
			oldElev, num(r["elevation_gain_m"]),
			oldDur, num(r["duration_minutes"]))
		changed++
	}

	fmt.Printf("\n%d reprocessed, %d skipped\n", changed, skipped)
	if changed == 0 {
		return
	}
	if o.dry {
		fmt.Println("(dry run — nothing saved)")
		return
	}
	saveBike(o, p)
	fmt.Printf("saved to %s\n", o.url)
}

// reprocessOne asks the server to re-parse one ride's stored GPX and returns the
// recomputed ride fields. It targets the GPX by file identity to avoid relying
// on the ride index staying stable server-side.
func reprocessOne(o opts, index int, gpxFile string) (map[string]interface{}, error) {
	body, contentType := multipartForm(map[string]string{
		"ride_index": strconv.Itoa(index),
		"gpx_file":   gpxFile,
	})
	req, _ := http.NewRequest("POST", o.url+"/admin/api/bike/reprocess-gpx", body)
	req.Header.Set("Content-Type", contentType)
	req.SetBasicAuth(o.user, o.pass)
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var res struct {
		Success bool                   `json:"success"`
		Error   string                 `json:"error"`
		Ride    map[string]interface{} `json:"ride"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if !res.Success {
		if res.Error == "" {
			res.Error = resp.Status
		}
		return nil, fmt.Errorf("%s", res.Error)
	}
	return res.Ride, nil
}

// saveBike persists the (mutated) bike plugin config back to the target. The
// admin plugin endpoint expects multipart/form-data with plugin/enabled/order
// and the settings map JSON-encoded as the "settings" field.
func saveBike(o opts, p pluginData) {
	settingsJSON, err := json.Marshal(p.Settings)
	check(err)
	body, contentType := multipartForm(map[string]string{
		"plugin":   "bike",
		"enabled":  strconv.FormatBool(p.Enabled),
		"order":    strconv.Itoa(p.Order),
		"settings": string(settingsJSON),
	})
	req, _ := http.NewRequest("POST", o.url+"/admin/api/plugin", body)
	req.Header.Set("Content-Type", contentType)
	req.SetBasicAuth(o.user, o.pass)
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	check(err)
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fatalf("save failed (%s): %s", resp.Status, strings.TrimSpace(string(rb)))
	}
	var res struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(rb, &res) == nil && !res.Success {
		fatalf("save failed: %s", res.Error)
	}
}

func multipartForm(fields map[string]string) (*bytes.Buffer, string) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	return buf, w.FormDataContentType()
}

func num(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}
