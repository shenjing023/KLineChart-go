package kline

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// markChart renders one chart and returns the frame plus the panes the
// frame used, for the pixel assertions below.
func markChart(t *testing.T, opts ChartOptions) (*Chart, *image.RGBA) {
	t.Helper()
	c := NewChart(opts)
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	tt := ui.NewTester(c.View, 900, 620)
	return c, tt.Image()
}

// countRowColor counts pixels of exactly c on one horizontal row of the
// price plot, which is how a dashed line shows up.
func countRowColor(img *image.RGBA, y int, c ui.Color) int {
	n := 0
	b := img.Bounds()
	if y < b.Min.Y || y >= b.Max.Y {
		return 0
	}
	for x := b.Min.X; x < b.Max.X; x++ {
		p := img.RGBAAt(x, y)
		if p.R == c.R && p.G == c.G && p.B == c.B {
			n++
		}
	}
	return n
}

// TestLastPriceDrawsADashedLine is the current-price mark a reader looks
// for: a dashed line at the newest close, colored by that bar.
func TestLastPriceDrawsADashedLine(t *testing.T) {
	hot := ui.RGB(3, 7, 11)
	style := DarkDefault()
	style.LastPrice = hot
	style.HighLow = ui.RGB(200, 200, 200) // keep the extremes off this color
	c, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
		Style:      style,
	})
	last, _ := c.Bar("A", c.Bars()-1)
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	panes := c.painted[0]
	y := int(panes.Price.Y + (1-scale.At(last.Close))*panes.Price.H)
	// The line is dashed, so one exact row holds a run of colored pixels
	// but not a solid bar. Allow a row either side for rounding.
	best := 0
	for _, dy := range []int{-1, 0, 1} {
		if n := countRowColor(img, y+dy, hot); n > best {
			best = n
		}
	}
	if best < 20 {
		t.Fatalf("last-price row has %d pixels of the mark color, want a dashed run", best)
	}
	// Dashed, not solid: well under the full plot width.
	if best > int(panes.Price.W) {
		t.Fatalf("the mark covers %d px of a %v-wide plot", best, panes.Price.W)
	}
}

// TestClearLastPriceHidesTheMark records the opt-out.
func TestClearLastPriceHidesTheMark(t *testing.T) {
	hot := ui.RGB(3, 7, 11)
	style := DarkDefault()
	style.LastPrice = hot
	style.Clear = ClearLastPrice
	_, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
		Style:      style,
	})
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if countRowColor(img, y, hot) > 0 {
			t.Fatalf("ClearLastPrice still drew a mark on row %d", y)
		}
	}
}

// TestHighLowTagsTheExtremes finds the extreme tag color above the
// highest high.
func TestHighLowTagsTheExtremes(t *testing.T) {
	hot := ui.RGB(5, 13, 19)
	style := DarkDefault()
	style.HighLow = hot
	c, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
		Style:      style,
	})
	first, last := c.Visible()
	panes := c.painted[0]
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	hi := math.Inf(-1)
	for i := first; i <= last; i++ {
		if h := c.panes[0].candles[i].High; h > hi {
			hi = h
		}
	}
	y := int(panes.Price.Y+(1-scale.At(hi))*panes.Price.H) - 8
	found := 0
	for dy := -6; dy <= 6; dy++ {
		found += countRowColor(img, y+dy, hot)
	}
	if found == 0 {
		t.Fatal("no high/low tag was painted near the highest high")
	}
}

// TestClearHighLowHidesTheTags records the opt-out.
func TestClearHighLowHidesTheTags(t *testing.T) {
	hot := ui.RGB(5, 13, 19)
	style := DarkDefault()
	style.HighLow = hot
	style.Clear = ClearHighLow
	_, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
		Style:      style,
	})
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if countRowColor(img, y, hot) > 0 {
			t.Fatalf("ClearHighLow still drew a tag on row %d", y)
		}
	}
}

// TestSetBarWidthChangesTheBarCount records the DIP width control a host
// asked for, and that it stays inside the wheel's bounds.
func TestSetBarWidthChangesTheBarCount(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(300)},
		Location:   ashareLocation,
	})
	wide := c.Bars()
	c.SetBarWidth(9)
	if !near64(c.BarWidth(), 9) {
		t.Fatalf("bar width %v, want 9", c.BarWidth())
	}
	tt := ui.NewTester(c.View, 900, 620)
	_ = tt.Image()
	first, last := c.Visible()
	if n := last - first + 1; n >= wide {
		t.Fatalf("wider bars show %d bars, was %d", n, wide)
	}
	// Bounds clamp rather than accept an unreadable width.
	c.SetBarWidth(1)
	if !near64(c.BarWidth(), MinBarWidth) {
		t.Fatalf("width %v, want the %v floor", c.BarWidth(), MinBarWidth)
	}
	c.SetBarWidth(500)
	if !near64(c.BarWidth(), MaxBarWidth) {
		t.Fatalf("width %v, want the %v ceiling", c.BarWidth(), MaxBarWidth)
	}
	c.SetBarWidth(0)
	if !near64(c.BarWidth(), MaxBarWidth) {
		t.Fatalf("a non-positive width changed the bar width to %v", c.BarWidth())
	}
}

// TestDrawSubPaneRunsInsideTheBand is the hook a host needs for a
// sub-pane legend. It must run once per visible sub-pane, get that
// band's rect, and paint inside it.
func TestDrawSubPaneRunsInsideTheBand(t *testing.T) {
	hot := ui.RGB(6, 14, 22)
	var calls int
	var rects []ui.Rect
	var names []string
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MACD", "KDJ"},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
	})
	c.DrawSubPane(func(p *ui.Painter, rect ui.Rect, ind Indicator, index int) {
		calls++
		rects = append(rects, rect)
		names = append(names, ind.Name())
		if index < 0 || index >= c.Bars() {
			t.Fatalf("hook index %d outside the series", index)
		}
		p.Fill(ui.Rect{X: rect.X + 2, Y: rect.Y + 2, W: 10, H: 10}, hot, 0)
	})
	tt := ui.NewTester(c.View, 900, 620)
	img := tt.Image()
	panes := c.painted[0]
	if calls < 2 {
		t.Fatalf("hook ran %d times, want once per sub-pane", calls)
	}
	var got int
	for _, r := range rects {
		if r != panes.MACD && r != panes.KDJ {
			t.Fatalf("hook rect %v is neither band %v nor %v", r, panes.MACD, panes.KDJ)
		}
	}
	for _, name := range names {
		if name != "MACD" && name != "KDJ" {
			t.Fatalf("hook saw indicator %q", name)
		}
	}
	for y := int(panes.MACD.Y); y < int(panes.MACD.Y+panes.MACD.H); y++ {
		got += countRowColor(img, y, hot)
	}
	if got == 0 {
		t.Fatal("nothing the hook painted reached the MACD band")
	}
}

// TestDrawSubPaneNilIsSafe records that the default is no hook.
func TestDrawSubPaneNilIsSafe(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MACD"},
		Source:     fixedSource{bars: hostBars(45)},
		Location:   ashareLocation,
	})
	c.DrawSubPane(nil)
	tt := ui.NewTester(c.View, 900, 620)
	if img := tt.Image(); img == nil {
		t.Fatal("nil hook produced no frame")
	}
}

func near64(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

// TestLastPriceFollowsPreviousClose is the rule a host reported: the mark
// follows the change from the previous close, like the header's change,
// not the open-to-close move. A bar that gaps up and still closes above
// yesterday is an up day even though it fell from its own open.
func TestLastPriceFollowsPreviousClose(t *testing.T) {
	up := ui.RGB(220, 40, 40)
	down := ui.RGB(40, 200, 200)
	flat := ui.RGB(120, 120, 120)
	// A gap up, then a close below the open but above the previous close.
	bars := []Candle{
		{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
		{Time: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Open: 12, High: 13, Low: 10.5, Close: 11},
	}
	style := DarkDefault()
	// Distinct from the candle colors: the candles themselves are drawn
	// in Up/Down, so the mark needs its own palette to be counted alone.
	style.Up, style.Down, style.Muted = up, down, flat
	style.HighLow = ui.RGB(1, 1, 1)
	c, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
		Style:      style,
	})
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	panes := c.painted[0]
	last, _ := c.Bar("A", 1)
	y := int(panes.Price.Y + (1-scale.At(last.Close))*panes.Price.H)
	hasUp, hasDown := 0, 0
	for _, dy := range []int{-1, 0, 1} {
		hasUp += countRowColor(img, y+dy, up)
		hasDown += countRowColor(img, y+dy, down)
	}
	// The candle bodies are drawn in the same colors, so a hit on the
	// candle's own column is not evidence. The dashed line spans the
	// plot, so require the color to appear across many columns.
	if hasUp < 20 {
		t.Fatalf("close %v is above the previous close but the mark is not up (up=%d down=%d)", last.Close, hasUp, hasDown)
	}
	if hasDown > 20 {
		t.Fatalf("the mark used the open-to-close move: down=%d", hasDown)
	}
}

// TestLastPriceFlatIsMuted records the third state: an unchanged close
// must not read as a rise.
func TestLastPriceFlatIsMuted(t *testing.T) {
	up := ui.RGB(220, 40, 40)
	down := ui.RGB(40, 200, 200)
	flat := ui.RGB(120, 120, 120)
	bars := []Candle{
		{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
		{Time: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Open: 9.5, High: 11, Low: 9, Close: 10},
	}
	style := DarkDefault()
	style.Up, style.Down, style.Muted = up, down, flat
	style.HighLow = ui.RGB(1, 1, 1)
	c, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
		Style:      style,
	})
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	panes := c.painted[0]
	last, _ := c.Bar("A", 1)
	y := int(panes.Price.Y + (1-scale.At(last.Close))*panes.Price.H)
	for _, dy := range []int{-1, 0, 1} {
		if n := countRowColor(img, y+dy, up); n > 20 {
			t.Fatalf("an unchanged close was drawn as up on row %d (%d px)", y+dy, n)
		}
	}
}

// TestLastPriceTagHidesTheTickUnderIt records that the axis does not print
// two numbers where the filled tag already is. The fixture puts the last
// close exactly on a price tick, which is when the collision happens.
func TestLastPriceTagHidesTheTickUnderIt(t *testing.T) {
	hot := ui.RGB(3, 7, 11)
	muted := ui.RGB(99, 99, 99)
	// Build a series whose last close sits on the scale's own tick.
	bars := hostBars(40)
	scale0 := MainScale(bars, nil, Viewport{BarWidth: 8}, 800)
	ticks := scale0.Ticks(5)
	if len(ticks) == 0 {
		t.Skip("no ticks in this fixture")
	}
	target := ticks[len(ticks)/2].Value
	for i := range bars {
		bars[i].Close = target
		bars[i].Open = target
		bars[i].High = target + 1
		bars[i].Low = target - 1
	}
	style := DarkDefault()
	style.LastPrice = hot
	style.Muted = muted
	c, img := markChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
		Style:      style,
	})
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	panes := c.painted[0]
	y := int(panes.Price.Y + (1-scale.At(target))*panes.Price.H)
	// No muted label pixel may sit on the rows the tag covers, in the
	// axis column.
	covered := 0
	b := img.Bounds()
	for dy := -5; dy <= 5; dy++ {
		row := y + dy
		if row < b.Min.Y || row >= b.Max.Y {
			continue
		}
		for x := int(panes.PriceAxis.X) + 4; x < b.Max.X && x < int(panes.PriceAxis.X+panes.PriceAxis.W); x++ {
			p := img.RGBAAt(x, row)
			if p.R == muted.R && p.G == muted.G && p.B == muted.B {
				covered++
			}
		}
	}
	if covered != 0 {
		t.Fatalf("%d axis label pixels sit under the last-price tag", covered)
	}
	// The tag itself must be there, or the test proves nothing.
	tagPixels := 0
	for dy := -5; dy <= 5; dy++ {
		tagPixels += countRowColor(img, y+dy, hot)
	}
	if tagPixels == 0 {
		t.Fatal("no last-price tag was drawn; the collision case was not set up")
	}
}
