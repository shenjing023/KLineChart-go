package kline

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// linkDays builds per-trading-day bars. skip names days the symbol does
// not trade, which is how a suspended stock looks: the market panel has
// those days and the stock does not.
func linkDays(start time.Time, n int, skip map[string]bool) []Candle {
	var out []Candle
	d := start.In(ashareLocation)
	for len(out) < n {
		key := d.Format("2006-01-02")
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday && !skip[key] {
			i := len(out)
			open := 100 + float64(i)
			out = append(out, Candle{
				Time: time.Date(d.Year(), d.Month(), d.Day(), 9, 30, 0, 0, ashareLocation),
				Open: open, High: open + 2, Low: open - 1, Close: open + 1,
				Volume: 1000,
			})
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

func linkChart(t *testing.T, bars []Candle) *Chart {
	t.Helper()
	c := NewChart(ChartOptions{
		Symbols:    []string{"STOCK"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
	})
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	tt := ui.NewTester(c.View, 640, 480)
	_ = tt.Image()
	return c
}

// TestCrosshairTimeSkipsSuspendedDays is the reason the link is by time:
// the stock has no bar on the suspended day, and the panel must still
// land on that day when the pointer is on the stock's neighbor.
func TestCrosshairTimeSkipsSuspendedDays(t *testing.T) {
	start := time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation)
	suspended := "2024-03-06"
	stock := linkDays(start, 40, map[string]bool{suspended: true})
	panel := linkDays(start, 45, nil)

	// The stock has no bar on the suspended day.
	for _, b := range stock {
		if b.Time.Format("2006-01-02") == suspended {
			t.Fatal("fixture still has the suspended day")
		}
	}
	stockChart := linkChart(t, stock)
	panelChart := linkChart(t, panel)

	// Point at the stock's bar on 2024-03-07, the day after the gap.
	var target time.Time
	for _, b := range stock {
		if b.Time.Format("2006-01-02") == "2024-03-07" {
			target = b.Time
		}
	}
	if target.IsZero() {
		t.Fatal("fixture is missing 2024-03-07")
	}
	if !stockChart.SetCrosshairTime(target) {
		t.Fatal("the stock refused a day it trades")
	}
	got, ok := stockChart.CrosshairTime()
	if !ok || !got.Equal(target) {
		t.Fatalf("stock crosshair %v ok %v, want %v", got, ok, target)
	}

	// The panel has a bar on the suspended day, so it must accept it.
	var suspendedAt time.Time
	for _, b := range panel {
		if b.Time.Format("2006-01-02") == suspended {
			suspendedAt = b.Time
		}
	}
	if suspendedAt.IsZero() {
		t.Fatal("panel is missing the suspended day")
	}
	if !panelChart.SetCrosshairTime(suspendedAt) {
		t.Fatal("the panel refused a day it trades")
	}
	got, ok = panelChart.CrosshairTime()
	if !ok || !got.Equal(suspendedAt) {
		t.Fatalf("panel crosshair %v ok %v, want %v", got, ok, suspendedAt)
	}
	if got.Equal(target) {
		t.Fatal("the two charts reported the same bar from different days")
	}
}

// TestSetCrosshairTimeRefusesADayOutsideTheSeries records the miss path:
// a host linking two charts must be able to tell that the other chart
// has nothing to show.
func TestSetCrosshairTimeRefusesADayOutsideTheSeries(t *testing.T) {
	bars := linkDays(time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation), 5, nil)
	c := linkChart(t, bars)
	if c.SetCrosshairTime(time.Date(2020, 1, 2, 9, 30, 0, 0, ashareLocation)) {
		t.Fatal("a day before the series was accepted")
	}
	if _, ok := c.CrosshairTime(); ok {
		t.Fatal("a refused time left a crosshair behind")
	}
	if c.SetCrosshairTime(time.Time{}) {
		t.Fatal("a zero time was accepted")
	}
}

// TestPlacedCrosshairSurvivesAFrame is the host path: the second chart
// has no pointer of its own, and the placed crosshair must still paint.
func TestPlacedCrosshairSurvivesAFrame(t *testing.T) {
	bars := linkDays(time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation), 120, nil)
	c := linkChart(t, bars)
	want := bars[3].Time
	if !c.SetCrosshairTime(want) {
		t.Fatal("placement refused")
	}
	// Frames with no pointer at all.
	for i := 0; i < 3; i++ {
		tt := ui.NewTester(c.View, 640, 480)
		_ = tt.Image()
	}
	got, ok := c.CrosshairTime()
	if !ok || !got.Equal(want) {
		t.Fatalf("placed crosshair %v ok %v after frames, want %v", got, ok, want)
	}
	// The readout follows the placed crosshair, which is what a host
	// draws its own header from.
	if out := c.Readout("STOCK"); out == "" || !contains(out, "Hover") {
		t.Fatalf("readout %q does not report a hovered bar", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestPointerTakesBackTheCrosshair records that a host placement is not
// sticky once the user moves the pointer on that chart.
func TestPointerTakesBackTheCrosshair(t *testing.T) {
	bars := linkDays(time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation), 120, nil)
	c := linkChart(t, bars)
	if !c.SetCrosshairTime(bars[3].Time) {
		t.Fatal("placement refused")
	}
	tt := ui.NewTester(c.View, 640, 480)
	_ = tt.Image()
	box := mustBox(t, tt, "STOCK chart")
	tt.Move(box.X+box.W/2, box.Y+24)
	_ = tt.Image()
	got, ok := c.CrosshairTime()
	if !ok {
		t.Fatal("the pointer cleared the crosshair instead of taking it")
	}
	if got.Equal(bars[3].Time) {
		t.Fatal("the pointer did not take the crosshair back from the host")
	}
}

// TestOnCrosshairChangeReportsAndClears pins the callback contract a host
// forwards between two charts.
func TestOnCrosshairChangeReportsAndClears(t *testing.T) {
	bars := linkDays(time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation), 120, nil)
	c := linkChart(t, bars)
	type event struct {
		symbol string
		index  int
		ok     bool
	}
	var events []event
	c.OnCrosshairChange(func(symbol string, index int, ok bool) {
		events = append(events, event{symbol, index, ok})
	})
	tt := ui.NewTester(c.View, 640, 480)
	_ = tt.Image()
	box := mustBox(t, tt, "STOCK chart")
	tt.Move(box.X+box.W/2, box.Y+24)
	_ = tt.Image()
	if len(events) == 0 {
		t.Fatal("the callback never ran on a hover")
	}
	last := events[len(events)-1]
	if !last.ok || last.symbol != "STOCK" || last.index < 0 {
		t.Fatalf("hover event %+v", last)
	}
	tt.Move(box.X+box.W+20, box.Y+24) // over the price axis
	_ = tt.Image()
	if len(events) < 2 {
		t.Fatalf("leaving the plot reported nothing, events %+v", events)
	}
	if end := events[len(events)-1]; end.ok {
		t.Fatalf("leaving the plot reported ok: %+v", end)
	}
}

// TestCrosshairHookLink is the end-to-end shape dapu uses: chart A's
// callback forwards the time to chart B, and both report the same day.
func TestCrosshairHookLink(t *testing.T) {
	start := time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation)
	a := linkChart(t, linkDays(start, 120, nil))
	b := linkChart(t, linkDays(start, 120, nil))
	a.OnCrosshairChange(func(_ string, index int, ok bool) {
		if !ok {
			b.ClearCrosshairTime()
			return
		}
		t, ok := a.CrosshairTime()
		if ok {
			b.SetCrosshairTime(t)
		}
		_ = index
	})
	tt := ui.NewTester(a.View, 640, 480)
	_ = tt.Image()
	box := mustBox(t, tt, "STOCK chart")
	tt.Move(box.X+box.W/2, box.Y+24)
	_ = tt.Image()
	at, okA := a.CrosshairTime()
	bt, okB := b.CrosshairTime()
	if !okA || !okB {
		t.Fatalf("linked crosshair missing: a %v b %v", okA, okB)
	}
	if !at.Equal(bt) {
		t.Fatalf("linked charts disagree: a %v b %v", at, bt)
	}
}

// TestVisibleTimeRoundTrip records the pan-link API.
func TestVisibleTimeRoundTrip(t *testing.T) {
	bars := linkDays(time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation), 120, nil)
	c := linkChart(t, bars)
	from, to, ok := c.VisibleTime()
	if !ok || !from.Before(to) {
		t.Fatalf("visible time %v..%v ok %v", from, to, ok)
	}
	// A different chart with more history must be able to adopt it.
	more := linkChart(t, linkDays(time.Date(2023, 1, 2, 0, 0, 0, 0, ashareLocation), 500, nil))
	if !more.SetVisibleTime(from, to) {
		t.Fatal("the second chart refused a range inside its series")
	}
	f2, t2, ok2 := more.VisibleTime()
	if !ok2 {
		t.Fatal("the second chart has no visible range after adopting one")
	}
	// The adopted window need not match bar for bar: the two series
	// differ in length, which is the whole point. It must overlap the
	// requested dates, though.
	if t2.Before(from) || f2.After(to) {
		t.Fatalf("adopted range %v..%v does not overlap %v..%v", f2, t2, from, to)
	}
}

// TestScrollToTimeParksTheEnd records the settlement case: the viewport
// ends on a chosen day instead of the newest bar, with the bars after it
// still off screen until the user drags.
func TestScrollToTimeParksTheEnd(t *testing.T) {
	bars := linkDays(time.Date(2024, 3, 4, 0, 0, 0, 0, ashareLocation), 250, nil)
	c := linkChart(t, bars)
	const target = 120
	if !c.ScrollToTime(bars[target].Time) {
		t.Fatal("scroll refused a day in the series")
	}
	_, last := c.Visible()
	// The target sits OverscrollBars in from the right edge, so the last
	// visible bar is that far past it.
	if last < target || last > target+OverscrollBars+1 {
		t.Fatalf("visible end %d, want %d..%d", last, target, target+OverscrollBars+1)
	}
	if last >= len(bars)-1 {
		t.Fatal("the viewport jumped to the newest bar instead of the target")
	}
	if c.ScrollToTime(time.Date(2020, 1, 2, 0, 0, 0, 0, ashareLocation)) {
		t.Fatal("scroll accepted a day outside the series")
	}
}

// TestLinkAPIsAreSafeWithoutLayout records that a chart that has never
// painted refuses rather than panics, which a host's first frame needs.
func TestLinkAPIsAreSafeWithoutLayout(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:    []string{"STOCK"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
	})
	if _, _, ok := c.VisibleTime(); ok {
		t.Fatal("a chart without layout reported a visible range")
	}
	if c.SetVisibleTime(time.Now(), time.Now()) {
		t.Fatal("a chart without layout adopted a range")
	}
	if c.ScrollToTime(time.Now()) {
		t.Fatal("a chart without layout scrolled")
	}
	// A nil *Chart is not usable, by contract: only Close accepts nil.
}
