package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

type visitorsData struct {
	TotalVisits  int64            `json:"total_visits"`
	TodayVisits  int64            `json:"today_visits"`
	TodayBots    int64            `json:"today_bots"`
	CurrentDay   string           `json:"current_day"`
	LastUpdate   time.Time        `json:"last_update"`
	DailyStats   map[string]int64 `json:"daily_stats"`
	DailyUnique  map[string]int64 `json:"daily_unique"`
	UniqueHashes []string         `json:"unique_hashes"`
	CountryStats map[string]int64 `json:"country_stats"`
	MonthlyStats map[string]int64 `json:"monthly_stats"`
}

type geoRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

var botMarkers = []string{
	"bot", "spider", "crawl", "slurp", "facebookexternal",
	"python-requests", "go-http-client", "curl/", "wget",
	"scrapy", "headless", "monitor", "uptime", "dataprovider",
}

func isPublic(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

func isBotUA(line string) bool {
	end := strings.LastIndexByte(line, '"')
	if end <= 0 {
		return false
	}
	start := strings.LastIndexByte(line[:end], '"')
	if start < 0 {
		return false
	}
	ua := strings.ToLower(line[start+1 : end])
	for _, m := range botMarkers {
		if strings.Contains(ua, m) {
			return true
		}
	}
	return false
}

func openLog(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return struct {
			io.Reader
			io.Closer
		}{gz, f}, nil
	}
	return f, nil
}

func streamCountries(paths []string, db *maxminddb.Reader, skipBots bool) (map[string]int64, int64, int64) {
	countryCounts := map[string]int64{}
	ipCache := map[string]string{}
	var total, resolved int64

	for _, p := range paths {
		rc, err := openLog(p)
		if err != nil {
			fmt.Printf("skip %s: %v\n", p, err)
			continue
		}
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			sp := strings.IndexByte(line, ' ')
			if sp <= 0 {
				continue
			}
			ipStr := line[:sp]
			if !isPublic(ipStr) {
				continue
			}
			if skipBots && isBotUA(line) {
				continue
			}
			total++

			cc, seen := ipCache[ipStr]
			if !seen {
				var rec geoRecord
				if err := db.Lookup(net.ParseIP(ipStr), &rec); err == nil {
					cc = strings.ToUpper(rec.Country.ISOCode)
				}
				ipCache[ipStr] = cc
			}
			if cc != "" {
				countryCounts[cc]++
				resolved++
			}
		}
		rc.Close()
	}
	return countryCounts, total, resolved
}

func loadCounts(path string) map[string]int64 {
	m := map[string]int64{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func saveCounts(path string, m map[string]int64) {
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, path)
	}
}

func main() {
	home, _ := os.UserHomeDir()
	dataFlag := flag.String("data", filepath.Join(home, "about", "data", "visitors.json"), "path to visitors.json")
	mmdbFlag := flag.String("mmdb", "dbip-city-lite.mmdb", "path to MMDB geo database")
	countsFlag := flag.String("counts", "country_counts.json", "intermediate raw per-country request counts")
	fromCounts := flag.Bool("from-counts", false, "skip log streaming, scale from existing counts file")
	target := flag.Int64("target", 90000, "scale country totals to sum to this (0 = raw counts)")
	skipBots := flag.Bool("nobots", false, "exclude requests with bot/crawler user agents")
	dryRun := flag.Bool("dry", false, "do not write visitors.json (still writes counts file)")
	flag.Parse()

	var countryCounts map[string]int64

	if *fromCounts {
		countryCounts = loadCounts(*countsFlag)
		if len(countryCounts) == 0 {
			fmt.Printf("no counts in %s\n", *countsFlag)
			os.Exit(1)
		}
		fmt.Printf("loaded %d countries from %s\n", len(countryCounts), *countsFlag)
	} else {
		logArgs := flag.Args()
		if len(logArgs) == 0 {
			fmt.Println("usage: backfill [flags] <access.log|-> [globs...]")
			flag.PrintDefaults()
			os.Exit(1)
		}
		var logPaths []string
		for _, a := range logArgs {
			if m, _ := filepath.Glob(a); len(m) > 0 {
				logPaths = append(logPaths, m...)
			} else {
				logPaths = append(logPaths, a)
			}
		}
		db, err := maxminddb.Open(*mmdbFlag)
		if err != nil {
			fmt.Printf("open mmdb %s: %v\n", *mmdbFlag, err)
			os.Exit(1)
		}
		defer db.Close()

		var total, resolved int64
		countryCounts, total, resolved = streamCountries(logPaths, db, *skipBots)
		fmt.Printf("counted %d requests, resolved %d, %d countries\n", total, resolved, len(countryCounts))
		saveCounts(*countsFlag, countryCounts)
		fmt.Printf("wrote raw counts to %s\n", *countsFlag)
	}

	var rawSum int64
	for _, n := range countryCounts {
		rawSum += n
	}
	if rawSum == 0 {
		fmt.Println("nothing resolved, aborting")
		os.Exit(1)
	}

	final := map[string]int64{}
	var finalSum int64
	if *target > 0 {
		scale := float64(*target) / float64(rawSum)
		for cc, n := range countryCounts {
			v := int64(math.Round(float64(n) * scale))
			if v > 0 {
				final[cc] = v
				finalSum += v
			}
		}
		fmt.Printf("scaled to target %d (actual sum %d)\n", *target, finalSum)
	} else {
		final = countryCounts
		finalSum = rawSum
	}

	type kv struct {
		cc string
		n  int64
	}
	var top []kv
	for cc, n := range final {
		top = append(top, kv{cc, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	fmt.Println("top countries:")
	for i, t := range top {
		if i >= 15 {
			break
		}
		fmt.Printf("  %s: %d (%.1f%%)\n", t.cc, t.n, 100*float64(t.n)/float64(finalSum))
	}

	if *dryRun {
		fmt.Println("dry run, visitors.json untouched")
		return
	}

	raw, err := os.ReadFile(*dataFlag)
	if err != nil {
		fmt.Printf("read %s: %v\n", *dataFlag, err)
		os.Exit(1)
	}
	var vd visitorsData
	if err := json.Unmarshal(raw, &vd); err != nil {
		fmt.Printf("parse %s: %v\n", *dataFlag, err)
		os.Exit(1)
	}
	vd.CountryStats = final

	out, _ := json.MarshalIndent(&vd, "", "  ")
	tmp := *dataFlag + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		fmt.Printf("write: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, *dataFlag); err != nil {
		fmt.Printf("rename: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d countries into %s\n", len(final), *dataFlag)
}
