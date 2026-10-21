package kline

import (
	"testing"
	"time"
)

// Marker stacking counts earlier markers on the same bar and side. The
// old implementation rescanned the prefix for every marker, so a bar
// carrying thousands of trades cost O(m²) on every append and live
// revision. The chart must stay linear, and the stack numbers must not
// change.
func TestMarkerStackingIsLinear(t *testing.T) {
	const marks = 5000
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{},
	})
	cs := Generate(2000, 1)
	c.panes[0] = c.mount(c.panes[0], cs, false, c.period)
	// Every marker on one bar: the worst case for the stacking pass.
	// Nanosecond offsets keep all of them inside the same hourly open
	// interval.
	at := cs[1000].Time
	in := make([]Marker, marks)
	for i := range in {
		side := SideBuy
		if i%3 == 0 {
			side = SideSell
		}
		in[i] = Marker{Time: at.Add(time.Duration(i) * time.Nanosecond), Side: side}
	}
	if err := c.SetMarkers("600519", in); err != nil {
		t.Fatal(err)
	}

	// The expected stack numbers, computed the straightforward way.
	want := make([]int, marks)
	if err := c.SetMarkers("600519", nil); err != nil {
		t.Fatal(err)
	}
	for i := range in {
		n := 0
		for j := 0; j < i; j++ {
			if in[j].Side == in[i].Side {
				n++
			}
		}
		want[i] = n
	}
	if err := c.SetMarkers("600519", in); err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if got := c.panes[0].marks[i].stack; got != want[i] {
			t.Fatalf("marker %d stack %d, want %d", i, got, want[i])
		}
	}

	// A live append reindexes every marker. Compare the cost at two
	// marker counts to pin linear growth without depending on whether
	// race instrumentation or a slow machine changes absolute timing.
	// A quadratic pass makes the 5x sample about 25x slower; linear
	// work stays near five times. The generous factor tolerates
	// scheduling noise without admitting O(m²).
	fullCost := markerAppendCost(t, marks, cs)
	smallCost := markerAppendCost(t, marks/5, cs)
	if fullCost > smallCost*12 {
		t.Fatalf("%d markers cost %v per append, %d cost %v; growth is not linear", marks, fullCost, marks/5, smallCost)
	}
	for i := range want {
		if got := c.panes[0].marks[i].stack; got != want[i] {
			t.Fatalf("after append marker %d stack %d, want %d", i, got, want[i])
		}
	}
}

// markerAppendCost builds a chart with n markers on one bar and returns
// the average cost of one live append, which reindexes them all.
func markerAppendCost(t *testing.T, n int, cs []Candle) time.Duration {
	t.Helper()
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{},
	})
	c.panes[0] = c.mount(c.panes[0], cloneCandles(cs), false, c.period)
	at := cs[1000].Time
	in := make([]Marker, n)
	for i := range in {
		side := SideBuy
		if i%3 == 0 {
			side = SideSell
		}
		in[i] = Marker{Time: at.Add(time.Duration(i) * time.Nanosecond), Side: side}
	}
	if err := c.SetMarkers("600519", in); err != nil {
		t.Fatal(err)
	}
	next := cs[len(cs)-1]
	next.Time = next.Time.Add(time.Hour)
	next.Close += 0.5
	const appends = 20
	started := time.Now()
	for i := 0; i < appends; i++ {
		next.Time = next.Time.Add(time.Hour)
		if err := c.AppendBar("600519", next); err != nil {
			t.Fatal(err)
		}
	}
	return time.Since(started) / appends
}

// A marker outside the loaded series stays out of the grid, and one
// negative bar index never takes part in stacking.
func TestMarkerStackingSkipsUnloadedBars(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{},
	})
	cs := Generate(20, 1)
	c.panes[0] = c.mount(c.panes[0], cs, false, c.period)
	early := cs[0].Time.Add(-48 * time.Hour)
	in := []Marker{
		{Time: early, Side: SideBuy},
		{Time: cs[5].Time, Side: SideBuy},
		{Time: early, Side: SideBuy},
		{Time: cs[5].Time, Side: SideBuy},
		{Time: cs[5].Time, Side: SideSell},
	}
	if err := c.SetMarkers("600519", in); err != nil {
		t.Fatal(err)
	}
	wantBar := []int{-1, 5, -1, 5, 5}
	wantStack := []int{0, 0, 0, 1, 0}
	for i := range in {
		if got := c.panes[0].marks[i].bar; got != wantBar[i] {
			t.Fatalf("marker %d bar %d, want %d", i, got, wantBar[i])
		}
		if got := c.panes[0].marks[i].stack; got != wantStack[i] {
			t.Fatalf("marker %d stack %d, want %d", i, got, wantStack[i])
		}
	}
	// Markers outside the series keep bar -1 and are never drawn.
	if err := c.SetMarkers("600519", []Marker{{Time: early, Side: SideBuy}}); err != nil {
		t.Fatal(err)
	}
	if c.panes[0].marks[0].bar != -1 {
		t.Fatalf("out-of-range marker bar %d", c.panes[0].marks[0].bar)
	}
	for i := range c.panes[0].marks {
		if c.panes[0].marks[i].bar < 0 && c.panes[0].marks[i].stack != 0 {
			t.Fatalf("marker %d outside the series has stack %d", i, c.panes[0].marks[i].stack)
		}
	}
}
