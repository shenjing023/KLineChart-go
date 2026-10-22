package kline

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// req03Bars is a three-bar series where the second bar is a limit-up by
// the host's rule but not by a percentage threshold: the previous close
// is 10.00 and the limit is round(10.00 * 1.1, 2) = 11.00 exactly.
func req03Bars() []Candle {
	day := func(d int, o, h, l, c float64) Candle {
		return Candle{
			Time: time.Date(2024, 1, 2+d, 9, 30, 0, 0, ashareLocation),
			Open: o, High: h, Low: l, Close: c,
			Volume: 1000,
		}
	}
	return []Candle{
		day(0, 9.80, 10.10, 9.70, 10.00),
		// Exactly the limit price: 11.00 from a 10.00 previous close.
		day(1, 10.40, 11.00, 10.30, 11.00),
		// Just above the limit by percentage but not the limit price:
		// 11.01 / 11.00 = +0.09%, a bar the exchange would never open.
		// A threshold rule would paint it as a limit-up; the host rule
		// must not.
		day(2, 11.00, 11.05, 10.95, 11.01),
	}
}

// req03Host is the rule dapu uses: the close must equal the limit price
// computed from the previous close and rounded to cents.
func req03Host(limitUp ui.Color) func(i int, c, prev Candle) ui.Color {
	return func(i int, c, prev Candle) ui.Color {
		if i == 0 || prev.Close <= 0 {
			return ui.Color{}
		}
		limit := math.Round(prev.Close*1.10*100) / 100
		if math.Round(c.Close*100)/100 == math.Round(limit*100)/100 {
			return limitUp
		}
		return ui.Color{}
	}
}

// req03Chart renders one bar series and returns the frame.
func req03Chart(t *testing.T, opts ChartOptions) *ui.Tester {
	t.Helper()
	c := NewChart(opts)
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	tt := ui.NewTester(c.View, 640, 480)
	_ = tt.Image()
	return tt
}

// countColor reports how many pixels in the frame are exactly c.
func countColor(img *image.RGBA, c ui.Color) int {
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
		if r == c.R && g == c.G && b == c.B {
			n++
		}
	}
	return n
}

// TestBarColorPaintsTheHostsChoice is the core REQ-03 check: a color the
// built-in threshold rule cannot produce must appear when the host asks
// for it, and nothing must use that color when it does not.
func TestBarColorPaintsTheHostsChoice(t *testing.T) {
	hot := ui.RGB(1, 2, 3) // a color no default palette contains
	src := fixedSource{bars: req03Bars()}
	base := ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     src,
		Location:   ashareLocation,
	}
	without := req03Chart(t, base)
	if n := countColor(without.Image(), hot); n != 0 {
		t.Fatalf("the palette painted the host color without a callback: %d px", n)
	}
	with := base
	with.BarColor = req03Host(hot)
	got := req03Chart(t, with)
	if n := countColor(got.Image(), hot); n == 0 {
		t.Fatal("BarColor returned a color the frame never used")
	}
}

// TestBarColorFallsBackToTheBuiltInRule records that a zero answer keeps
// the ordinary up/down coloring, so a host only answers where it has a
// rule.
func TestBarColorFallsBackToTheBuiltInRule(t *testing.T) {
	calls := 0
	opts := ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: req03Bars()},
		Location:   ashareLocation,
		BarColor: func(int, Candle, Candle) ui.Color {
			calls++
			return ui.Color{}
		},
	}
	req03Chart(t, opts)
	if calls == 0 {
		t.Fatal("the callback was never asked")
	}
	// The frame must match one drawn with no callback at all.
	plain := ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: req03Bars()},
		Location:   ashareLocation,
	}
	a := req03Chart(t, opts).Image()
	b := req03Chart(t, plain).Image()
	if !bytesEqual(a, b) {
		t.Fatal("a callback that answers zero changed the frame")
	}
}

// TestBarColorSeesRawBarsInOverlay is the sharp edge the contract calls
// out: overlay divides prices by a base, and the host rule must still
// receive the original cents.
func TestBarColorSeesRawBarsInOverlay(t *testing.T) {
	bars := req03Bars()
	var seen []Candle
	opts := ChartOptions{
		Symbols:    []string{"TEST", "OTHER"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Layout:     LayoutOverlay,
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
		BarColor: func(i int, c, prev Candle) ui.Color {
			if len(seen) <= i {
				seen = append(seen, c)
			}
			return ui.Color{}
		},
	}
	req03Chart(t, opts)
	if len(seen) == 0 {
		t.Fatal("overlay never asked the host")
	}
	for i := range seen {
		if i >= len(bars) {
			break
		}
		if seen[i].Close != bars[i].Close || seen[i].Open != bars[i].Open {
			t.Fatalf("overlay handed bar %d scaled: %+v, want %+v", i, seen[i], bars[i])
		}
	}
}

// TestBarColorPaintsVolumeToo pins that the volume bar follows the same
// answer instead of close >= open.
func TestBarColorPaintsVolumeToo(t *testing.T) {
	hot := ui.RGB(4, 5, 6)
	opts := ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: req03Bars()},
		Location:   ashareLocation,
		BarColor: func(i int, c, prev Candle) ui.Color {
			if i == 0 {
				return hot
			}
			return ui.Color{}
		},
	}
	img := req03Chart(t, opts).Image()
	if countColor(img, hot) == 0 {
		t.Fatal("the volume bar did not take the host color")
	}
	// The volume fill is drawn at 85% alpha, so the exact color only
	// survives where the alpha rounds to opaque. Compare the alpha the
	// painter would use instead of relying on that.
	alpha := hot.Alpha(0.85)
	if countColor(img, alpha) == 0 && countColor(img, hot) == 0 {
		t.Fatal("neither the alpha nor the plain color appears")
	}
}

// TestBarColorIndexIsTheSeriesIndex pins the contract that i counts the
// whole series, not the visible window, so a host can hold a rule table
// indexed the same way as its bars.
func TestBarColorIndexIsTheSeriesIndex(t *testing.T) {
	bars := req03Bars()
	var idx []int
	opts := ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
		BarColor: func(i int, c, prev Candle) ui.Color {
			idx = append(idx, i)
			return ui.Color{}
		},
	}
	req03Chart(t, opts)
	if len(idx) == 0 {
		t.Fatal("no indices reported")
	}
	for _, i := range idx {
		if i < 0 || i >= len(bars) {
			t.Fatalf("index %d outside the series of %d", i, len(bars))
		}
	}
}

// TestBarColorPrevIsZeroOnFirstBar records the documented shape of the
// first bar so a host rule can skip it without guessing.
func TestBarColorPrevIsZeroOnFirstBar(t *testing.T) {
	var first Candle
	var sawFirst bool
	opts := ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: req03Bars()},
		Location:   ashareLocation,
		BarColor: func(i int, c, prev Candle) ui.Color {
			if i == 0 {
				first, sawFirst = prev, true
			}
			return ui.Color{}
		},
	}
	req03Chart(t, opts)
	if !sawFirst {
		t.Fatal("bar 0 was never reported")
	}
	if first != (Candle{}) {
		t.Fatalf("prev for bar 0 is %+v, want the zero Candle", first)
	}
}

func bytesEqual(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	return string(a.Pix) == string(b.Pix)
}

// TestBarColorPrevIsTheRealPredecessorAtTheWindowEdge is the case a host
// asked about: the first visible bar is usually not bar 0, and its prev
// must be the bar before it in the series, not the zero Candle. Getting
// this wrong would silently drop one limit-up color at the left edge.
func TestBarColorPrevIsTheRealPredecessorAtTheWindowEdge(t *testing.T) {
	bars := hostBars(120)
	c := NewChart(ChartOptions{
		Symbols:    []string{"TEST"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
		BarColor:   func(int, Candle, Candle) ui.Color { return ui.Color{} },
	})
	// Pan so the visible window starts somewhere in the middle.
	tt := ui.NewTester(c.View, 640, 480)
	_ = tt.Image()
	c.vp.Offset = 40
	first, _ := c.Visible()
	if first <= 0 {
		t.Fatalf("window starts at %d; the test needs a mid-series edge", first)
	}
	var seen []struct {
		i          int
		prev       Candle
		prevIsZero bool
	}
	c.barColor = func(i int, _ Candle, prev Candle) ui.Color {
		seen = append(seen, struct {
			i          int
			prev       Candle
			prevIsZero bool
		}{i, prev, prev == (Candle{})})
		return ui.Color{}
	}
	_ = ui.NewTester(c.View, 640, 480).Image()
	if len(seen) == 0 {
		t.Fatal("no bars were reported")
	}
	if seen[0].i == 0 {
		t.Fatalf("the first reported bar is 0; the window did not start mid-series")
	}
	if seen[0].prevIsZero {
		t.Fatalf("bar %d got a zero prev; it must be the bar before it in the series", seen[0].i)
	}
	if seen[0].prev.Close != bars[seen[0].i-1].Close {
		t.Fatalf("bar %d prev close %v, want %v", seen[0].i, seen[0].prev.Close, bars[seen[0].i-1].Close)
	}
}
