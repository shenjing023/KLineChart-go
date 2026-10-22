package kline

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// axisChart builds a chart with a known price precision and volume
// formatter so the axis labels are easy to assert on.
func axisChart(t *testing.T, opts ChartOptions) (*Chart, *ui.Tester) {
	t.Helper()
	if opts.Symbols == nil {
		opts.Symbols = []string{"A"}
	}
	if opts.Period.Span == 0 {
		opts.Period = Period{Span: 1, Unit: UnitDay}
	}
	if opts.Indicators == nil {
		opts.Indicators = []string{}
	}
	if opts.Source == nil {
		opts.Source = fixedSource{bars: hostBars(80)}
	}
	if opts.Location == nil {
		opts.Location = ashareLocation
	}
	c := NewChart(opts)
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	tt := ui.NewTester(c.View, 900, 620)
	_ = tt.Image()
	return c, tt
}

// TestPriceDecimalsReachesTheAxis is the gap the M0 probe found: a host
// asked for whole numbers on a 0AMV panel and the axis still read
// 4200.00 while its own header read 4200.
func TestPriceDecimalsReachesTheAxis(t *testing.T) {
	zero := 0
	c, _ := axisChart(t, ChartOptions{PriceDecimals: &zero})
	// Ask the chart's own paint path for the scale it drew, not a copy
	// with the format applied by hand: the bug this pins is exactly that
	// paint did not apply it.
	scale := paintedPriceScale(t, c)
	if scale.Format == nil {
		t.Fatal("the painted price scale has no formatter")
	}
	ticks := scale.Ticks(5)
	if len(ticks) == 0 {
		t.Fatal("no price ticks")
	}
	for _, tick := range ticks {
		if strings.Contains(tick.Label, ".") {
			t.Fatalf("axis label %q keeps decimals under PriceDecimals=0", tick.Label)
		}
	}
}

// paintedPriceScale renders one frame with a probe that captures the
// scale drawPrice returns.
func paintedPriceScale(t *testing.T, a *Chart) chart.Linear {
	t.Helper()
	var got chart.Linear
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			panes := Split(r)
			got = drawPrice(p, panes, a.panes[0].candles, nil, a.vp, Pointer{}, -1,
				barPaint{style: DarkDefault()}, a.labels())
		})
	}, 900, 620)
	_ = tt.Image()
	return got
}

// TestPriceDecimalsTwoKeepsFractionalLabels is the other half: the
// default must not lose the decimals a stock price needs.
func TestPriceDecimalsTwoKeepsFractionalLabels(t *testing.T) {
	c, _ := axisChart(t, ChartOptions{})
	labels := c.labels()
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	scale.Format = labels.price
	sawFraction := false
	for _, tick := range scale.Ticks(5) {
		if tick.Value != float64(int64(tick.Value)) {
			sawFraction = true
			if !strings.Contains(tick.Label, ".") {
				t.Fatalf("axis label %q dropped a fractional price", tick.Label)
			}
		}
	}
	if !sawFraction {
		t.Skip("this fixture has whole-number ticks only")
	}
}

// TestFormatVolumeReplacesAxisText is REQ-13: the host quotes volume in
// 万手 and the axis text follows, without moving a tick.
func TestFormatVolumeReplacesAxisText(t *testing.T) {
	c, _ := axisChart(t, ChartOptions{
		FormatVolume: func(float64) string { return "WAN" },
	})
	// The tester reports only widget text, not text the painter draws,
	// so the assertion goes through the scale drawPrice configures.
	first, last := c.Visible()
	ticks := volumeScale(c.panes[0].candles, first, last, c.labels()).Ticks(2)
	if len(ticks) == 0 {
		t.Fatal("no volume ticks")
	}
	if got := ticks[len(ticks)-1].Label; got != "WAN" {
		t.Fatalf("volume tick label %q, want the host text", got)
	}
	// The price axis is untouched: its labels are still numbers.
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	scale.Format = c.labels().price
	for _, tick := range scale.Ticks(5) {
		if strings.Contains(tick.Label, "WAN") {
			t.Fatalf("the volume formatter leaked onto the price axis: %q", tick.Label)
		}
	}
}

// TestFormatTickSeparatesAxisFromCrosshair is the fix for a blind mask
// that turned every monthly axis tick into the same string: the axis can
// carry a coarser label than the crosshair.
func TestFormatTickSeparatesAxisFromCrosshair(t *testing.T) {
	seen := map[TickKind]int{}
	c, tt := axisChart(t, ChartOptions{
		FormatTick: func(_ time.Time, kind TickKind) string {
			seen[kind]++
			switch kind {
			case TickAxis:
				return "AXIS"
			case TickCrosshair:
				return "CROSS"
			default:
				return "READ"
			}
		},
	})
	if seen[TickAxis] == 0 {
		t.Fatal("the axis was never formatted")
	}
	labels := c.labels()
	if got := labels.at(time.Now(), TickAxis, ""); got != "AXIS" {
		t.Fatalf("axis role formatted as %q", got)
	}
	if got := labels.at(time.Now(), TickCrosshair, ""); got != "CROSS" {
		t.Fatalf("crosshair role formatted as %q", got)
	}
	box := mustBox(t, tt, "A chart")
	tt.Move(box.X+box.W/2, box.Y+24)
	_ = tt.Image()
	if seen[TickCrosshair] == 0 {
		t.Fatal("the crosshair was never formatted")
	}
	c.crossIndex = 3
	if out := c.Readout("A"); !strings.Contains(out, "READ") {
		t.Fatalf("readout %q does not use FormatTick", out)
	}
}

// TestFormatTickKeepsTickPositions records that a per-role formatter,
// like the plain one, moves nothing.
func TestFormatTickKeepsTickPositions(t *testing.T) {
	a, _ := axisChart(t, ChartOptions{})
	b, _ := axisChart(t, ChartOptions{
		FormatTick: func(time.Time, TickKind) string { return "X" },
	})
	if a.vp != b.vp {
		t.Fatalf("viewport changed under FormatTick: %+v vs %+v", a.vp, b.vp)
	}
	af, al := a.Visible()
	bf, bl := b.Visible()
	if af != bf || al != bl {
		t.Fatalf("visible range changed: %d..%d vs %d..%d", af, al, bf, bl)
	}
}

// TestPriceAxisLabelCollisionIsDropped is the overlap the probe
// screenshot showed: the price axis's lowest label and the volume axis's
// top label printed in the same place. labelNear decides it, and this
// pins that a tick too close to a price tick reports a collision.
func TestPriceAxisLabelCollisionIsDropped(t *testing.T) {
	c, _ := axisChart(t, ChartOptions{})
	panes := c.painted[0]
	if panes.Volume.H <= 8 {
		t.Skip("no volume band in this layout")
	}
	scale := MainScale(c.panes[0].candles, nil, c.vp, c.plotW)
	// The price plot's bottom edge is the volume strip's top edge, so a
	// label there must be reported as colliding with the lowest price
	// tick.
	bottom := panes.Price.Y + panes.Price.H
	if !nearAnyPriceTick(scale, panes.Price, bottom) {
		t.Fatal("labelNear missed a label sitting on the price axis's lowest tick")
	}
	// A label in the middle of the volume strip is far from the price
	// ticks and must not be suppressed.
	mid := panes.Volume.Y + panes.Volume.H/2
	if nearAnyPriceTick(scale, panes.Price, mid) {
		t.Fatal("labelNear suppressed a volume label that has room")
	}
}

// nearAnyPriceTick is labelNear's rule, written out for the test so a
// change to the helper's constant is caught here.
func nearAnyPriceTick(scale chart.Linear, price ui.Rect, y float32) bool {
	const labelH = 14 // one 11-DIP label plus a little air
	for _, tick := range scale.Ticks(5) {
		if tick.At < -0.01 || tick.At > 1.01 {
			continue
		}
		ty := chart.Y(price, tick.At)
		if d := ty - y; d < labelH && d > -labelH {
			return true
		}
	}
	return false
}

// TestMinTimeAxisHeightIsHonest keeps the exported constant usable: at
// that height every band a host expects is present and worth looking at.
// The constant is a floor for hosts, not the layout's internal cutoff, so
// it must be at or above what the split needs and must leave a readable
// chart, not just a mathematically present strip.
func TestMinTimeAxisHeightIsHonest(t *testing.T) {
	for _, tc := range []struct {
		name string
		macd bool
		kdj  bool
	}{
		{"price and volume", false, false},
		{"one sub-pane", true, false},
		{"two sub-panes", true, true},
	} {
		p := SplitStack(ui.Rect{W: 600, H: MinTimeAxisHeight}, true, tc.macd, tc.kdj)
		if p.Time.H <= 0 {
			t.Fatalf("%s: the time axis is gone at MinTimeAxisHeight", tc.name)
		}
		if p.Price.H < 30 {
			t.Fatalf("%s: price plot is only %v DIP at MinTimeAxisHeight", tc.name, p.Price.H)
		}
		if p.Volume.H < 8 {
			t.Fatalf("%s: volume strip is only %v DIP at MinTimeAxisHeight", tc.name, p.Volume.H)
		}
	}
}

// TestPriceDecimalsReachesPriceDimensionIndicators closes an
// inconsistency a host found in the readout: with PriceDecimals=0 the
// OHLC prices printed as whole numbers while MA on the same line still
// read "MA5 188.00".
func TestPriceDecimalsReachesPriceDimensionIndicators(t *testing.T) {
	for _, tc := range []struct {
		digits int
		want   string
	}{
		{0, "MA5 188"},    // whole numbers
		{2, "MA5 188.00"}, // the default
		{3, "MA5 188.000"},
	} {
		d := tc.digits
		c := hostChart(t, ChartOptions{
			Symbols:       []string{"A"},
			Period:        Period{Span: 1, Unit: UnitDay},
			Indicators:    []string{"MA"},
			Source:        fixedSource{bars: hostBars(90)},
			Location:      ashareLocation,
			PriceDecimals: &d,
		})
		out := c.Readout("A")
		if !strings.Contains(out, tc.want) {
			t.Fatalf("digits %d: readout %q, want %q", tc.digits, out, tc.want)
		}
	}
}

// TestPriceDecimalsLeavesIndexIndicatorsAlone is the other half: KDJ is
// not a price, so 90.91 must not become 91 when a panel asks for whole
// number prices.
func TestPriceDecimalsLeavesIndexIndicatorsAlone(t *testing.T) {
	d := 0
	c := hostChart(t, ChartOptions{
		Symbols:       []string{"A"},
		Period:        Period{Span: 1, Unit: UnitDay},
		Indicators:    []string{"KDJ"},
		Source:        fixedSource{bars: hostBars(90)},
		Location:      ashareLocation,
		PriceDecimals: &d,
	})
	out := c.Readout("A")
	if !strings.Contains(out, "K 90.91") {
		t.Fatalf("KDJ lost its precision under PriceDecimals=0: %q", out)
	}
}

// TestPriceScaleIsOptional records that a host indicator without
// SetPriceDigits keeps its own formatting and is never asked for one.
func TestPriceScaleIsOptional(t *testing.T) {
	var ma Indicator = NewMA(5)
	if _, ok := ma.(PriceScale); !ok {
		t.Fatal("MA should expose SetPriceDigits")
	}
	var k Indicator = NewKDJ(9, 3, 3)
	if _, ok := k.(PriceScale); ok {
		t.Fatal("KDJ must not expose SetPriceDigits: its values are not prices")
	}
	applyPriceDigits(k, 0) // must not panic or change anything
}

// TestAxisLabelsHugThePlotNotTheCardEdge records the placement a
// reference A-share chart uses: the scale numbers sit immediately right
// of the plot, and the filled tags (last price, crosshair) are the ones
// that reach the card edge. Right-aligning the ticks left a band of empty
// column between the candles and their numbers.
func TestAxisLabelsHugThePlotNotTheCardEdge(t *testing.T) {
	muted := ui.RGB(99, 99, 99)
	style := DarkDefault()
	style.Muted = muted
	style.LastPrice = ui.RGB(1, 2, 3)
	style.HighLow = ui.RGB(4, 5, 6)
	c, tt := axisChart(t, ChartOptions{Style: style, Indicators: []string{"MA"}})
	img := tt.Image()
	panes := c.painted[0]
	b := img.Bounds()
	leftmost := -1
	for y := int(panes.PriceAxis.Y); y < int(panes.PriceAxis.Y+panes.PriceAxis.H); y++ {
		limit := int(panes.PriceAxis.X + panes.PriceAxis.W)
		if limit > b.Max.X {
			limit = b.Max.X
		}
		for x := int(panes.PriceAxis.X); x < limit; x++ {
			p := img.RGBAAt(x, y)
			if p.R == muted.R && p.G == muted.G && p.B == muted.B {
				if leftmost < 0 || x < leftmost {
					leftmost = x
				}
			}
		}
	}
	if leftmost < 0 {
		t.Fatal("no axis label was painted")
	}
	// The first glyph must start within an inset or two of the plot's
	// right edge; a wider gap reads as a gutter.
	if gap := leftmost - int(panes.PriceAxis.X); gap > 8 {
		t.Fatalf("the first axis label starts %d DIP right of the plot; ticks hug the plot", gap)
	}
}

// TestFilledTagsAnchorAtTheAxisStart records the other half: the filled
// readouts (last price, crosshair) are what sit against the axis edge, so
// their box starts at the axis column rather than being centred on it.
func TestFilledTagsAnchorAtTheAxisStart(t *testing.T) {
	fill := ui.RGB(7, 11, 13)
	style := DarkDefault()
	style.LastPrice = fill
	style.HighLow = ui.RGB(4, 5, 6)
	c, tt := axisChart(t, ChartOptions{Style: style, Indicators: []string{}})
	img := tt.Image()
	panes := c.painted[0]
	b := img.Bounds()
	// Find the filled tag: the leftmost pixel of the fill color inside
	// the axis column.
	leftmost, rightmost := -1, -1
	for y := int(panes.PriceAxis.Y); y < int(panes.PriceAxis.Y+panes.PriceAxis.H) && y < b.Max.Y; y++ {
		limit := int(panes.PriceAxis.X + panes.PriceAxis.W)
		if limit > b.Max.X {
			limit = b.Max.X
		}
		for x := int(panes.PriceAxis.X); x < limit; x++ {
			p := img.RGBAAt(x, y)
			if p.R == fill.R && p.G == fill.G && p.B == fill.B {
				if leftmost < 0 || x < leftmost {
					leftmost = x
				}
				if x > rightmost {
					rightmost = x
				}
			}
		}
	}
	if leftmost < 0 {
		t.Fatal("no filled tag was painted")
	}
	// It starts at the axis column, i.e. right against the plot edge.
	if gap := leftmost - int(panes.PriceAxis.X); gap > 2 {
		t.Fatalf("the filled tag starts %d DIP into the axis; it should hug the axis", gap)
	}
}
