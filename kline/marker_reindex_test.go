package kline

import (
	"testing"
	"time"
)

// Marker stacking counts earlier markers on the same bar and side. The
// regression ensures thousands of trades keep insertion order and stack
// numbers through SetMarkers and an append-driven reindex. It is a value
// test, not proof of linear complexity.
func TestMarkerStackingLargeInput(t *testing.T) {
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

	// Reindexing records a side counter for each bar. These values keep the
	// reindex path correct after appends; this test does not measure whether
	// the pass is linear.
	c.fitted = true
	c.plotW = 800
	next := cs[len(cs)-1]
	for range 3 {
		next.Time = next.Time.Add(time.Hour)
		next.Close += 0.5
		if err := c.AppendBar("600519", next); err != nil {
			t.Fatal(err)
		}
	}
	for i := range want {
		if got := c.panes[0].marks[i].stack; got != want[i] {
			t.Fatalf("after append marker %d stack %d, want %d", i, got, want[i])
		}
	}
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
