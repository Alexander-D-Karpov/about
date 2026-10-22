package plugins

import (
	"testing"
	"time"
)

func TestIsLoopbackIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.0.0.1":      true,
		"::1":            true,
		"88.201.217.189": false,
		"172.17.0.1":     false,
		"":               false,
		"garbage":        false,
	} {
		if got := isLoopbackIP(ip); got != want {
			t.Errorf("isLoopbackIP(%q) = %v, want %v", ip, got, want)
		}
	}
}

// The trend starts at the first day with data instead of padding the window with
// empty bars, and is scaled by unique visitors so one polling client can't dwarf
// every other day.
func TestBuildTrendStartsAtFirstDataDayAndUsesUnique(t *testing.T) {
	p := newVisitorsForTest(t.TempDir())
	today := time.Now()
	day := func(n int) string { return today.AddDate(0, 0, -n).Format("2006-01-02") }

	p.currentDay = day(0)
	p.todayCount = 700
	p.uniqueToday = map[string]struct{}{"a": {}, "b": {}}
	p.dailyStats[day(1)] = 720 // poller-inflated visits
	p.dailyUnique[day(1)] = 4
	p.dailyStats[day(2)] = 10
	p.dailyUnique[day(2)] = 8

	tr := p.buildTrendLocked(30)
	if tr == nil {
		t.Fatal("expected a trend")
	}
	if len(tr.Bars) != 3 {
		t.Fatalf("got %d bars, want 3 (first data day .. today)", len(tr.Bars))
	}
	if tr.Bars[0].Height != 100 {
		t.Errorf("busiest unique day should be full height, got %d", tr.Bars[0].Height)
	}
	if tr.Bars[1].Height != 50 {
		t.Errorf("4 of 8 unique should be 50%%, got %d", tr.Bars[1].Height)
	}
	if !tr.Bars[2].Today {
		t.Error("last bar should be today")
	}
	if tr.TrendDir != "" {
		t.Errorf("no full previous week, trend should be hidden, got %q", tr.TrendDir)
	}
}

func TestBuildTrendEmptyHistory(t *testing.T) {
	p := newVisitorsForTest(t.TempDir())
	p.currentDay = time.Now().Format("2006-01-02")
	if tr := p.buildTrendLocked(30); tr != nil {
		t.Fatalf("expected no trend without data, got %+v", tr)
	}
}
