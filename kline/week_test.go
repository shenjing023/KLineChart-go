package kline

import (
	"errors"
	"math"
	"testing"
	"time"
)

// weekZone is the calendar the dapu contract names for weekly buckets.
var weekZone = time.FixedZone("Asia/Shanghai", 8*60*60)

// weekBars builds one weekday bar per local day from start, so a test
// can lay out a calendar without a market calendar.
func weekBars(start time.Time, days int) []Candle {
	out := make([]Candle, 0, days)
	d := start.In(weekZone)
	for i := 0; i < days; i++ {
		for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			d = d.AddDate(0, 0, 1)
		}
		open := 100 + float64(i)
		out = append(out, Candle{
			Time:   time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, weekZone),
			Open:   open,
			High:   open + 2,
			Low:    open - 1,
			Close:  open + 1,
			Volume: 1000 + float64(i),
		})
		d = d.AddDate(0, 0, 1)
	}
	return out
}

// TestWeekPeriodIsValidButNotLoadable pins the two answers a week gives:
// it is a real period, and no source serves it.
func TestWeekPeriodIsValidButNotLoadable(t *testing.T) {
	p := Period{Span: 1, Unit: UnitWeek}
	if !p.Valid() {
		t.Fatal("1w should be a valid period")
	}
	if p.Loadable() {
		t.Fatal("1w must not be loadable: only the chart aggregates it")
	}
	d, err := p.Duration()
	if err != nil || d != 7*24*time.Hour {
		t.Fatalf("1w duration %v err %v", d, err)
	}
	if got := p.String(); got != "1w" {
		t.Fatalf("1w label %q", got)
	}
}

// TestWeekBucketOpenIsMonday is the rule the contract names: the bucket
// is named by the first base bar of the local Monday week.
func TestWeekBucketOpenIsMonday(t *testing.T) {
	// 2024-12-30 is a Monday; four bars Mon-Thu land in one bucket.
	bars := weekBars(time.Date(2024, 12, 30, 0, 0, 0, 0, weekZone), 4)
	p := Period{Span: 1, Unit: UnitWeek}
	for i := range bars {
		got := bucketOpen(bars, i, p, weekZone)
		if !got.Equal(bars[0].Time) {
			t.Fatalf("bar %d bucket open %v, want %v", i, got, bars[0].Time)
		}
	}
	// The next Monday starts a new bucket.
	next := weekBars(time.Date(2025, 1, 6, 0, 0, 0, 0, weekZone), 1)
	got := bucketOpen(next, 0, p, weekZone)
	if !got.Equal(next[0].Time) {
		t.Fatalf("new week open %v, want %v", got, next[0].Time)
	}
}

// TestWeekBucketSpansNewYear is the case a year-truncated bucket would
// get wrong. 2025-01-01 is a Wednesday, so 2024-12-30 (Mon) and
// 2025-01-03 (Fri) belong to one week that crosses the year boundary.
func TestWeekBucketSpansNewYear(t *testing.T) {
	bars := weekBars(time.Date(2024, 12, 30, 0, 0, 0, 0, weekZone), 5)
	if len(bars) != 5 {
		t.Fatalf("want five weekday bars, got %d", len(bars))
	}
	if bars[0].Time.Year() != 2024 || bars[4].Time.Year() != 2025 {
		t.Fatalf("fixture should cross the year: %v .. %v", bars[0].Time, bars[4].Time)
	}
	p := Period{Span: 1, Unit: UnitWeek}
	agg := aggregate(bars, p, weekZone)
	if len(agg) != 1 {
		t.Fatalf("a week that crosses New Year is one bucket, got %d: %+v", len(agg), agg)
	}
	w := agg[0]
	if !w.Time.Equal(bars[0].Time) {
		t.Fatalf("week opens on the Monday, got %v want %v", w.Time, bars[0].Time)
	}
	if w.Open != bars[0].Open || w.Close != bars[4].Close {
		t.Fatalf("week open/close %v/%v, want %v/%v", w.Open, w.Close, bars[0].Open, bars[4].Close)
	}
	wantHigh, wantLow, wantVol := bars[0].High, bars[0].Low, 0.0
	for _, b := range bars {
		wantHigh = math.Max(wantHigh, b.High)
		wantLow = math.Min(wantLow, b.Low)
		wantVol += b.Volume
	}
	if w.High != wantHigh || w.Low != wantLow || math.Abs(w.Volume-wantVol) > 1e-9 {
		t.Fatalf("week h/l/v %v/%v/%v, want %v/%v/%v", w.High, w.Low, w.Volume, wantHigh, wantLow, wantVol)
	}
}

// TestWeekAggregateMatchesDailyRule cross-checks the aggregate against an
// independent walk over the same bars: same bucket count, same OHLCV.
func TestWeekAggregateMatchesDailyRule(t *testing.T) {
	bars := weekBars(time.Date(2024, 11, 4, 0, 0, 0, 0, weekZone), 60)
	for i := range bars {
		bars[i].Volume = float64(1000+i) + 0.25
	}
	p := Period{Span: 1, Unit: UnitWeek}
	got := aggregate(bars, p, weekZone)
	want := independentWeeks(t, bars)
	if len(got) != len(want) {
		t.Fatalf("week count %d, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if !g.Time.Equal(w.Time) || g.Open != w.Open || g.High != w.High || g.Low != w.Low || g.Close != w.Close {
			t.Fatalf("week %d = %+v, want %+v", i, g, w)
		}
		if math.Abs(g.Volume-w.Volume) > 1e-9 {
			t.Fatalf("week %d volume %v, want %v", i, g.Volume, w.Volume)
		}
	}
}

// independentWeeks is a second implementation of the rule, written from
// the contract text rather than from the library code, so a shared bug
// in the library cannot make both agree.
func independentWeeks(t *testing.T, bars []Candle) []Candle {
	t.Helper()
	var out []Candle
	for _, b := range bars {
		local := b.Time.In(weekZone)
		monday := local.AddDate(0, 0, -int(local.Weekday()+6)%7)
		monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 9, 30, 0, 0, weekZone)
		if n := len(out); n > 0 && out[n-1].Time.Equal(monday) {
			cur := &out[n-1]
			cur.High = math.Max(cur.High, b.High)
			cur.Low = math.Min(cur.Low, b.Low)
			cur.Close = b.Close
			cur.Volume += b.Volume
			continue
		}
		out = append(out, Candle{Time: monday, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume})
	}
	return out
}

// req01Chart starts a replay over a daily series so week aggregation is
// reachable: a plain load cannot ask a source for a week.
func req01Chart(t *testing.T, bars []Candle) (*Chart, *Replay) {
	t.Helper()
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
	// SeekIndex reveals the first i bars, so len(bars) reveals them all.
	if err := rp.SeekIndex(len(bars)); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if err := c.SetPeriod(Period{Span: 1, Unit: UnitWeek}); err != nil {
		t.Fatalf("weekly period: %v", err)
	}
	return c, rp
}

// fixedSource serves one prepared series for any init request.
type fixedSource struct{ bars []Candle }

func (s fixedSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if req.Kind == KindInit {
		return append([]Candle(nil), s.bars...), false, nil
	}
	return nil, false, nil
}

// TestReplayWeekAggregatesRevealedPrefix is the core replay rule: the
// weekly series holds only the revealed prefix, and the last bucket is
// the forming one.
func TestReplayWeekAggregatesRevealedPrefix(t *testing.T) {
	bars := weekBars(time.Date(2024, 11, 4, 0, 0, 0, 0, weekZone), 40)
	c, rp := req01Chart(t, bars)
	revealed := rp.Cursor()
	want := aggregate(bars[:revealed], Period{Span: 1, Unit: UnitWeek}, weekZone)
	if got := c.Bars(); got != len(want) {
		t.Fatalf("weekly bars %d, want %d", got, len(want))
	}
	for i := range want {
		bar, ok := c.Bar("600519", i)
		if !ok {
			t.Fatalf("missing weekly bar %d", i)
		}
		if !bar.Time.Equal(want[i].Time) || bar.Open != want[i].Open || bar.Close != want[i].Close {
			t.Fatalf("weekly bar %d = %+v, want %+v", i, bar, want[i])
		}
	}
	// The hidden suffix must not reach the weekly series.
	for _, w := range want {
		if w.Time.After(bars[revealed-1].Time) {
			t.Fatalf("weekly bar %v is beyond the revealed prefix", w.Time)
		}
	}
}

// TestReplayWeekRevisesLastBucket is the incremental rule from the dapu
// fixture: revealing one more day changes only the last weekly bar.
func TestReplayWeekRevisesLastBucket(t *testing.T) {
	bars := weekBars(time.Date(2024, 11, 4, 0, 0, 0, 0, weekZone), 40)
	c, rp := req01Chart(t, bars)
	// Step back until the next bar to reveal is a Tuesday, so the reveal
	// joins the week that is already on screen instead of opening one.
	// Cursor is the revealed count, so bars[Cursor] is the next one.
	// Cursor == len is "everything revealed", so bars[Cursor] is only a
	// valid next bar after at least one step back.
	for rp.Cursor() > 0 && (rp.Cursor() >= len(bars) || bars[rp.Cursor()].Time.In(weekZone).Weekday() != time.Tuesday) {
		if err := rp.StepBack(1); err != nil {
			t.Fatalf("step back: %v", err)
		}
	}
	if rp.Cursor() <= 0 || rp.Cursor() >= len(bars) {
		t.Fatal("never found a Tuesday to reveal")
	}
	before := snapshotBars(t, c)
	if len(before) == 0 {
		t.Fatal("no weekly bars to compare")
	}
	if err := rp.Step(1); err != nil {
		t.Fatalf("step: %v", err)
	}
	after := snapshotBars(t, c)
	// Same number of buckets: the revealed day joined the forming week.
	if len(after) != len(before) {
		t.Fatalf("bucket count changed %d -> %d; a same-week reveal must revise", len(before), len(after))
	}
	for i := 0; i < len(after)-1; i++ {
		if after[i] != before[i] {
			t.Fatalf("week %d changed, only the last bucket may move: %+v -> %+v", i, before[i], after[i])
		}
	}
	if after[len(after)-1] == before[len(before)-1] {
		t.Fatal("the last weekly bar did not update after a reveal")
	}
}

func snapshotBars(t *testing.T, c *Chart) []Candle {
	t.Helper()
	out := make([]Candle, c.Bars())
	for i := range out {
		b, ok := c.Bar("600519", i)
		if !ok {
			t.Fatalf("missing bar %d", i)
		}
		out[i] = b
	}
	return out
}

// TestLastBarFormingFollowsTheCalendar pins the new answer a host draws
// its "in progress" frame from.
func TestLastBarFormingFollowsTheCalendar(t *testing.T) {
	bars := weekBars(time.Date(2024, 11, 4, 0, 0, 0, 0, weekZone), 40)
	c, rp := req01Chart(t, bars)
	// Seek just past each bar and check the flag against the calendar.
	// SeekIndex takes a revealed count, so i + 1 reveals bars[i] last.
	for i := 0; i < len(bars); i++ {
		if err := rp.SeekIndex(i + 1); err != nil {
			t.Fatalf("seek %d: %v", i, err)
		}
		wd := bars[i].Time.In(weekZone).Weekday()
		want := wd != time.Friday && wd != time.Saturday && wd != time.Sunday
		if got := c.LastBarForming(); got != want {
			t.Fatalf("reveal %d on %v: LastBarForming %v, want %v", i, wd, got, want)
		}
	}
}

// TestLastBarFormingFalseOffReplay records that the flag is about an
// unfinished bucket, not about live updates.
func TestLastBarFormingFalseOffReplay(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
	})
	if c.LastBarForming() {
		t.Fatal("a plain chart has no forming bucket")
	}
	var nilChart *Chart
	if nilChart.LastBarForming() {
		t.Fatal("a nil chart reports nothing forming")
	}
}

// TestWeekIsNotAFinerReplayPeriod records that a week is coarser than a
// day, so it is allowed during a replay and is not ErrReplayPeriod.
func TestWeekIsNotAFinerReplayPeriod(t *testing.T) {
	bars := weekBars(time.Date(2024, 11, 4, 0, 0, 0, 0, weekZone), 30)
	c, rp := req01Chart(t, bars)
	if err := c.SetPeriod(Period{Span: 1, Unit: UnitDay}); err != nil {
		t.Fatalf("back to daily: %v", err)
	}
	if c.Bars() != rp.Cursor() {
		t.Fatalf("daily bars %d, want %d", c.Bars(), rp.Cursor())
	}
	if c.LastBarForming() {
		t.Fatal("a daily replay bar is never forming")
	}
}

// TestWeekPlainLoadIsRefused records that a week cannot be loaded. A
// host that asks anyway gets ErrInvalidPeriod and keeps its bars.
func TestWeekPlainLoadIsRefused(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{},
	})
	before := c.Bars()
	err := c.SetPeriod(Period{Span: 1, Unit: UnitWeek})
	if !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("weekly load err %v, want ErrInvalidPeriod", err)
	}
	if c.Bars() != before || c.Period() != (Period{Span: 1, Unit: UnitHour}) {
		t.Fatalf("a refused load changed the chart: %d bars, period %v", c.Bars(), c.Period())
	}
}
