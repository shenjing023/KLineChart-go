package kline

import (
	"errors"
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNewChartFrameNonEmpty(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Layout:     LayoutStack,
		Indicators: []string{"MA", "MACD"},
	})
	tt := ui.NewTester(c.View, 960, 720)
	img := tt.Image()
	if img == nil || img.Bounds().Empty() {
		t.Fatal("tester frame is empty")
	}
	if !frameHasInk(img) {
		t.Fatal("tester frame has no painted pixels")
	}
	if !tt.HasText("ALPHA") || !tt.HasText("BETA") {
		t.Fatalf("headers missing, texts %q", tt.Texts())
	}
	if len(c.painted) != 2 {
		t.Fatalf("stack panes %d", len(c.painted))
	}
	for i, panes := range c.painted {
		if panes.Price.H <= 1 || panes.Volume.H <= 1 || panes.MACD.H <= 1 {
			t.Fatalf("pane %d wants candles, volume, and MACD: %+v", i, panes)
		}
	}
	if !c.IndicatorOn("MA") || !c.IndicatorOn("MACD") || c.IndicatorOn("BOLL") || c.IndicatorOn("KDJ") {
		t.Fatalf("indicator defaults MA=%v MACD=%v BOLL=%v KDJ=%v", c.IndicatorOn("MA"), c.IndicatorOn("MACD"), c.IndicatorOn("BOLL"), c.IndicatorOn("KDJ"))
	}

	top := mustBox(t, tt, "ALPHA chart")
	x, y := top.X+top.W/2, top.Y+24
	start := c.vp.Offset
	width := c.vp.BarWidth
	tt.Press(x, y)
	tt.Move(x+120, y)
	tt.Release(x+120, y)
	if c.vp.Offset >= start-1 {
		t.Fatalf("pan offset %v, start %v", c.vp.Offset, start)
	}
	tt.Scroll(x, y, 0, -100)
	if c.vp.BarWidth <= width {
		t.Fatalf("zoom width %v, was %v", c.vp.BarWidth, width)
	}
	tt.Move(x, y)
	if c.crossIndex < 0 || !tt.HasText("Hover") {
		t.Fatalf("crosshair index %d texts %q", c.crossIndex, tt.Texts())
	}
}

func TestSetPeriodIsSafe(t *testing.T) {
	c := NewChart(ChartOptions{})
	tt := ui.NewTester(c.View, 960, 720)
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if c.Period().String() != "4h" || c.Bars() == 0 || tt.Image() == nil || !frameHasInk(tt.Image()) {
		t.Fatalf("period %s bars %d", c.Period(), c.Bars())
	}
	if err := c.SetPeriod(Period{}); err == nil {
		t.Fatal("invalid period should return an error")
	}
	tt.Frame()
	if c.Period().String() != "4h" || tt.Image() == nil {
		t.Fatal("a rejected period should leave the chart paintable")
	}
	if err := c.SetPeriod(Period{Span: 1, Unit: UnitDay}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(c.panes[0].candles) < 2 {
		t.Fatal("day period did not load")
	}
	if got := c.panes[0].candles[1].Time.Sub(c.panes[0].candles[0].Time); got != 24*time.Hour {
		t.Fatalf("step %s", got)
	}
}

func TestSetSourceRejectsNilAndSwaps(t *testing.T) {
	c := NewChart(ChartOptions{})
	if err := c.SetSource(nil); err != ErrNilSource {
		t.Fatalf("nil source: %v", err)
	}
	stub := &StubLiveSource{}
	bars := append([]Candle(nil), c.panes[0].candles...)
	if err := stub.SetBars("ALPHA", c.Period(), bars); err != nil {
		t.Fatal(err)
	}
	if err := stub.SetBars("BETA", c.Period(), c.panes[1].candles); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.src.(LiveSource); !ok {
		t.Fatal("SetSource should keep the live source")
	}
	bad := sourceFunc(func(BarRequest) ([]Candle, bool, error) {
		return nil, false, ErrInvalidPeriod
	})
	if err := c.SetSource(bad); err == nil {
		t.Fatal("failed reload should return the source error")
	}
	if _, ok := c.src.(*StubLiveSource); !ok {
		t.Fatal("failed SetSource should restore the previous source")
	}
}

func TestSetStyleAndLimitPct(t *testing.T) {
	c := NewChart(ChartOptions{Indicators: []string{}})
	if c.LimitPct() != 0 {
		t.Fatalf("default limit %v", c.LimitPct())
	}
	st := DarkDefault()
	st.Up = ui.RGB(255, 0, 200)
	st.Down = ui.RGB(0, 180, 255)
	st.LimitUp = ui.RGB(255, 180, 255)
	st.LimitDown = ui.RGB(180, 255, 255)
	st.HollowUp = false
	st.LimitPct = 0.10
	c.SetStyle(st)
	if c.Hollow() || c.LimitPct() != 0.10 {
		t.Fatalf("hollow %v limit %v", c.Hollow(), c.LimitPct())
	}
	tt := ui.NewTester(c.View, 960, 720)
	if !c.styleSet || c.chartStyle(nil).Up != st.Up {
		t.Fatal("SetStyle should override theme candle colors")
	}
	up := countNear(tt.Image(), c.painted[0].Price, st.Up)
	if up < 20 {
		t.Fatalf("custom up pixels %d", up)
	}
	c.SetHollow(false)
	c.SetLimitPct(0)
	tt.Frame()
	if c.Hollow() || c.LimitPct() != 0 {
		t.Fatal("hollow and limit switches should stay live after SetStyle")
	}
	if c.chartStyle(nil).Up != st.Up || c.chartStyle(nil).Down != st.Down {
		t.Fatal("clearing hollow and limit should keep the color override")
	}
	c.SetLimitPct(-1)
	if c.LimitPct() != 0 {
		t.Fatalf("negative limit stored %v", c.LimitPct())
	}
}

func TestSetLayoutRejectsUnknown(t *testing.T) {
	c := NewChart(ChartOptions{})
	if err := c.SetLayout(LayoutOverlay); err != nil {
		t.Fatal(err)
	}
	if c.Layout() != LayoutOverlay {
		t.Fatalf("layout %s", c.Layout())
	}
	if err := c.SetLayout("sideways"); err == nil {
		t.Fatal("unknown layout returned nil")
	} else if !errors.Is(err, ErrUnknownLayout) {
		t.Fatalf("unknown layout: %v", err)
	}
	if c.Layout() != LayoutOverlay {
		t.Fatal("unknown layout changed the chart")
	}
	if err := c.SetLayout(LayoutStack); err != nil {
		t.Fatal(err)
	}
	if c.Layout() != LayoutStack {
		t.Fatalf("layout %s", c.Layout())
	}
}

func frameHasInk(img *image.RGBA) bool {
	if img == nil {
		return false
	}
	b := img.Bounds()
	var r, g, bl uint32
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			c := img.RGBAAt(x, y)
			r += uint32(c.R)
			g += uint32(c.G)
			bl += uint32(c.B)
			n++
		}
	}
	if n == 0 {
		return false
	}
	// A flat fill is one color. Candles and grid move the average off
	// the first pixel.
	first := img.RGBAAt(b.Min.X, b.Min.Y)
	return r/uint32(n) != uint32(first.R) || g/uint32(n) != uint32(first.G) || bl/uint32(n) != uint32(first.B)
}
