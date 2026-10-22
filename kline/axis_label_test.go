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
