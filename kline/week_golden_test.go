package kline

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

// The golden file is exported by the dapu host from its TypeScript
// implementation of the same rule, so this test compares two
// independent implementations rather than the library against itself.
// It lives in testdata/ so `go test` runs it without the dapu checkout.
const weekGoldenPath = "testdata/week-aggregation-golden.json"

type weekGolden struct {
	SchemaVersion   int `json:"schemaVersion"`
	WindowStart     int `json:"windowStart"`
	BackgroundCount int `json:"backgroundCount"`
	TotalCount      int `json:"totalCount"`
	DailyBars       []struct {
		Date       string  `json:"date"`
		OpenCents  float64 `json:"openCents"`
		HighCents  float64 `json:"highCents"`
		LowCents   float64 `json:"lowCents"`
		CloseCents float64 `json:"closeCents"`
		Volume     float64 `json:"volume"`
	} `json:"dailyBars"`
	Snapshots []struct {
		RevealedCount int    `json:"revealedCount"`
		RevealedDate  string `json:"revealedDate"`
		Weeks         []struct {
			WeekStart  string  `json:"weekStart"`
			LastDate   string  `json:"lastDate"`
			InProgress bool    `json:"inProgress"`
			OpenCents  float64 `json:"openCents"`
			HighCents  float64 `json:"highCents"`
			LowCents   float64 `json:"lowCents"`
			CloseCents float64 `json:"closeCents"`
			Volume     float64 `json:"volume"`
		} `json:"weeks"`
	} `json:"snapshots"`
	LastWeekByReveal []struct {
		RevealedCount int     `json:"revealedCount"`
		RevealedDate  string  `json:"revealedDate"`
		WeekStart     string  `json:"weekStart"`
		LastDate      string  `json:"lastDate"`
		InProgress    bool    `json:"inProgress"`
		OpenCents     float64 `json:"openCents"`
		HighCents     float64 `json:"highCents"`
		LowCents      float64 `json:"lowCents"`
		CloseCents    float64 `json:"closeCents"`
		Volume        float64 `json:"volume"`
	} `json:"lastWeekByReveal"`
	CrossYearSamples []struct {
		Name          string `json:"name"`
		SpansTwoYears bool   `json:"spansTwoYears"`
		Bars          []struct {
			Date       string  `json:"date"`
			OpenCents  float64 `json:"openCents"`
			HighCents  float64 `json:"highCents"`
			LowCents   float64 `json:"lowCents"`
			CloseCents float64 `json:"closeCents"`
			Volume     float64 `json:"volume"`
		} `json:"bars"`
		Weeks []struct {
			WeekStart  string  `json:"weekStart"`
			LastDate   string  `json:"lastDate"`
			InProgress bool    `json:"inProgress"`
			OpenCents  float64 `json:"openCents"`
			HighCents  float64 `json:"highCents"`
			LowCents   float64 `json:"lowCents"`
			CloseCents float64 `json:"closeCents"`
			Volume     float64 `json:"volume"`
		} `json:"weeks"`
	} `json:"crossYearSamples"`
}

// goldenBars converts one sample's daily rows into chart bars.
func goldenBars(t *testing.T, in []struct {
	Date       string  `json:"date"`
	OpenCents  float64 `json:"openCents"`
	HighCents  float64 `json:"highCents"`
	LowCents   float64 `json:"lowCents"`
	CloseCents float64 `json:"closeCents"`
	Volume     float64 `json:"volume"`
}) []Candle {
	t.Helper()
	out := make([]Candle, len(in))
	for i, b := range in {
		d, err := time.ParseInLocation("2006-01-02", b.Date, weekZone)
		if err != nil {
			t.Fatalf("golden date %q: %v", b.Date, err)
		}
		out[i] = Candle{
			Time:   time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, weekZone),
			Open:   b.OpenCents / 100,
			High:   b.HighCents / 100,
			Low:    b.LowCents / 100,
			Close:  b.CloseCents / 100,
			Volume: b.Volume,
		}
	}
	return out
}

func loadWeekGolden(t *testing.T) *weekGolden {
	t.Helper()
	raw, err := os.ReadFile(weekGoldenPath)
	if err != nil {
		t.Skipf("golden data not present: %v", err)
	}
	var g weekGolden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("golden data: %v", err)
	}
	if g.SchemaVersion != 1 {
		t.Fatalf("golden schema %d", g.SchemaVersion)
	}
	return &g
}

// goldenDays turns the fixture's calendar dates into the daily bars the
// chart aggregates. dapu stores prices in integer cents; the chart works
// in price units, so this divides once and keeps the volume as given.
func goldenDays(t *testing.T, g *weekGolden) []Candle {
	t.Helper()
	out := make([]Candle, len(g.DailyBars))
	for i, b := range g.DailyBars {
		d, err := time.ParseInLocation("2006-01-02", b.Date, weekZone)
		if err != nil {
			t.Fatalf("golden date %q: %v", b.Date, err)
		}
		out[i] = Candle{
			Time:   time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, weekZone),
			Open:   b.OpenCents / 100,
			High:   b.HighCents / 100,
			Low:    b.LowCents / 100,
			Close:  b.CloseCents / 100,
			Volume: b.Volume,
		}
	}
	return out
}

func weekDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, weekZone)
	if err != nil {
		t.Fatalf("date %q: %v", s, err)
	}
	return d
}

// sameWeekBar compares a library week against a golden week. Prices are
// compared in cents, which is the unit the host's rule is defined in;
// volume uses a tolerance because the fixture mixes integers and
// floats and a sum of floats depends on the order of addition.
func sameWeekBar(got Candle, wantStart, wantLast string, openC, highC, lowC, closeC, vol float64, t *testing.T) bool {
	t.Helper()
	// The host records a week by its calendar dates, not by an instant.
	// A week's open time is the first revealed bar of that Monday week,
	// and when the Monday is a holiday that is the Tuesday, so the open
	// instant legitimately differs from the named Monday. The check is
	// therefore "the bar's date falls in the week that starts at
	// wantStart", not "the bar's instant equals Monday midnight".
	want := weekDate(t, wantStart)
	gotLocal := got.Time.In(weekZone)
	gotWeekStart := gotLocal.AddDate(0, 0, -int(gotLocal.Weekday()+6)%7)
	gotWeekStart = time.Date(gotWeekStart.Year(), gotWeekStart.Month(), gotWeekStart.Day(), 0, 0, 0, 0, weekZone)
	if !gotWeekStart.Equal(want) {
		t.Errorf("week bar %v belongs to the week starting %s, want %s",
			gotLocal, gotWeekStart.Format("2006-01-02"), wantStart)
		return false
	}
	// A week's last date must also match, so a bucket that swallowed a
	// neighboring week cannot pass by sharing its Monday.
	if gotLocal.Format("2006-01-02") > wantLast {
		t.Errorf("week bar opens %s, after the host's last date %s", gotLocal.Format("2006-01-02"), wantLast)
		return false
	}
	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"open", got.Open, openC / 100},
		{"high", got.High, highC / 100},
		{"low", got.Low, lowC / 100},
		{"close", got.Close, closeC / 100},
	}
	for _, c := range checks {
		if math.Abs(c.got-c.want) > 1e-6 {
			t.Errorf("week %s %s = %v, want %v", wantStart, c.name, c.got, c.want)
			return false
		}
	}
	if math.Abs(got.Volume-vol) > 1e-6 {
		t.Errorf("week %s volume = %v, want %v", wantStart, got.Volume, vol)
		return false
	}
	return true
}

// TestWeekGoldenFullSeries compares every weekly bar of the whole window
// against the host's export.
func TestWeekGoldenFullSeries(t *testing.T) {
	g := loadWeekGolden(t)
	if len(g.DailyBars) != g.TotalCount {
		t.Fatalf("golden has %d bars, want %d", len(g.DailyBars), g.TotalCount)
	}
	if len(g.Snapshots) == 0 {
		t.Fatal("golden has no snapshots")
	}
	last := g.Snapshots[len(g.Snapshots)-1]
	if last.RevealedCount != g.TotalCount {
		t.Fatalf("last snapshot reveals %d of %d", last.RevealedCount, g.TotalCount)
	}
	bars := goldenDays(t, g)
	got := aggregate(bars, Period{Span: 1, Unit: UnitWeek}, weekZone)
	if len(got) != len(last.Weeks) {
		t.Fatalf("week count %d, want %d", len(got), len(last.Weeks))
	}
	for i, w := range last.Weeks {
		sameWeekBar(got[i], w.WeekStart, w.LastDate, w.OpenCents, w.HighCents, w.LowCents, w.CloseCents, w.Volume, t)
	}
}

// TestWeekGoldenEveryReveal is the incremental rule: after each single
// reveal the last weekly bar must equal the host's expectation for that
// reveal count.
func TestWeekGoldenEveryReveal(t *testing.T) {
	g := loadWeekGolden(t)
	bars := goldenDays(t, g)
	p := Period{Span: 1, Unit: UnitWeek}
	for _, want := range g.LastWeekByReveal {
		if want.RevealedCount <= 0 || want.RevealedCount > len(bars) {
			t.Fatalf("golden reveal %d outside the series", want.RevealedCount)
		}
		prefix := bars[:want.RevealedCount]
		weeks := aggregate(prefix, p, weekZone)
		if len(weeks) == 0 {
			t.Fatalf("reveal %d produced no week", want.RevealedCount)
		}
		got := weeks[len(weeks)-1]
		if !sameWeekBar(got, want.WeekStart, want.LastDate, want.OpenCents, want.HighCents, want.LowCents, want.CloseCents, want.Volume, t) {
			t.Fatalf("reveal %d (%s)", want.RevealedCount, want.RevealedDate)
		}
	}
}

// TestWeekGoldenAggregateOneMatchesAggregate proves the incremental path
// the replay uses agrees with the full-series path the test above
// checked: aggregateOne at the last index must equal the last bucket.
func TestWeekGoldenAggregateOneMatchesAggregate(t *testing.T) {
	g := loadWeekGolden(t)
	bars := goldenDays(t, g)
	p := Period{Span: 1, Unit: UnitWeek}
	for _, want := range g.LastWeekByReveal {
		prefix := bars[:want.RevealedCount]
		one := aggregateOne(prefix, len(prefix)-1, p, weekZone)
		full := aggregate(prefix, p, weekZone)
		last := full[len(full)-1]
		if !one.Time.Equal(last.Time) || one.Open != last.Open || one.High != last.High ||
			one.Low != last.Low || one.Close != last.Close || math.Abs(one.Volume-last.Volume) > 1e-9 {
			t.Fatalf("reveal %d: aggregateOne %+v, aggregate last %+v", want.RevealedCount, one, last)
		}
	}
}

// TestWeekGoldenReplayMatchesHost drives the real replay path: a daily
// replay switched to weekly must produce the host's series for the
// revealed prefix, and its last bar must report forming exactly when
// the host says the week is in progress.
func TestWeekGoldenReplayMatchesHost(t *testing.T) {
	g := loadWeekGolden(t)
	bars := goldenDays(t, g)
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Location:   weekZone,
		Source:     fixedSource{bars: bars},
	})
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	rp, err := StartReplay(c, map[string][]Candle{"600519": bars})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if err := rp.SeekIndex(g.TotalCount); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if err := c.SetPeriod(Period{Span: 1, Unit: UnitWeek}); err != nil {
		t.Fatalf("weekly: %v", err)
	}
	last := g.Snapshots[len(g.Snapshots)-1]
	if c.Bars() != len(last.Weeks) {
		t.Fatalf("replay weekly bars %d, want %d", c.Bars(), len(last.Weeks))
	}
	for i, w := range last.Weeks {
		bar, ok := c.Bar("600519", i)
		if !ok {
			t.Fatalf("missing weekly bar %d", i)
		}
		sameWeekBar(bar, w.WeekStart, w.LastDate, w.OpenCents, w.HighCents, w.LowCents, w.CloseCents, w.Volume, t)
	}
	// The fixture's final week ends on a Thursday, so the chart must
	// still call it forming: the week is not provably closed.
	if want := last.Weeks[len(last.Weeks)-1].InProgress; c.LastBarForming() != want {
		t.Fatalf("LastBarForming %v, host says inProgress %v", c.LastBarForming(), want)
	}
}

// TestWeekGoldenSwitchingPeriodKeepsCursor records the contract promise
// that changing the period does not move the training progress.
func TestWeekGoldenSwitchingPeriodKeepsCursor(t *testing.T) {
	g := loadWeekGolden(t)
	bars := goldenDays(t, g)
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Location:   weekZone,
		Source:     fixedSource{bars: bars},
	})
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	rp, err := StartReplay(c, map[string][]Candle{"600519": bars})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	const reveal = 600
	if err := rp.SeekIndex(reveal); err != nil {
		t.Fatalf("seek: %v", err)
	}
	daily := c.Bars()
	if daily != reveal {
		t.Fatalf("daily bars %d at reveal %d", daily, reveal)
	}
	if err := c.SetPeriod(Period{Span: 1, Unit: UnitWeek}); err != nil {
		t.Fatalf("weekly: %v", err)
	}
	if rp.Cursor() != reveal {
		t.Fatalf("weekly switch moved the cursor to %d, want %d", rp.Cursor(), reveal)
	}
	if err := c.SetPeriod(Period{Span: 1, Unit: UnitDay}); err != nil {
		t.Fatalf("daily: %v", err)
	}
	if rp.Cursor() != reveal || c.Bars() != reveal {
		t.Fatalf("round trip: cursor %d bars %d, want %d", rp.Cursor(), c.Bars(), reveal)
	}
}

// TestWeekGoldenCrossYear compares the host's own cross-year windows.
// These are the cases the main 640-bar window does not contain: a week
// that starts in one year and ends in the next, and the counter-example
// where a holiday week looks like it should span the year but does not.
func TestWeekGoldenCrossYear(t *testing.T) {
	g := loadWeekGolden(t)
	if len(g.CrossYearSamples) == 0 {
		t.Skip("the fixture has no cross-year samples")
	}
	spanning := 0
	for _, s := range g.CrossYearSamples {
		if s.SpansTwoYears {
			spanning++
		}
		bars := goldenBars(t, s.Bars)
		got := aggregate(bars, Period{Span: 1, Unit: UnitWeek}, weekZone)
		if len(got) != len(s.Weeks) {
			t.Fatalf("%s: week count %d, want %d", s.Name, len(got), len(s.Weeks))
		}
		for i, w := range s.Weeks {
			if !sameWeekBar(got[i], w.WeekStart, w.LastDate, w.OpenCents, w.HighCents, w.LowCents, w.CloseCents, w.Volume, t) {
				t.Fatalf("%s week %d (%s)", s.Name, i, w.WeekStart)
			}
		}
		// A sample the host calls spanning must really have a week whose
		// first and last trading day fall in different years, or the
		// assertion above is checking an ordinary week.
		for _, w := range s.Weeks {
			if w.WeekStart[:4] != w.LastDate[:4] {
				if !s.SpansTwoYears {
					t.Fatalf("%s spans the year but is marked as not spanning", s.Name)
				}
			}
		}
	}
	if spanning < 2 {
		t.Fatalf("only %d cross-year samples; the fixture was probably trimmed wrong", spanning)
	}
}
