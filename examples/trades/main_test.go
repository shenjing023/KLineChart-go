package main

import (
	"image"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

func tradesTester(t *testing.T) (*tradesDemo, *ui.Tester) {
	t.Helper()
	d, err := newTradesDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.chart.Close(); err != nil {
			t.Error(err)
		}
	})
	return d, ui.NewTester(tradesView(d), 960, 700)
}

func tradeGeometry(t *testing.T, d *tradesDemo, tt *ui.Tester) (kline.Panes, kline.Viewport, chart.Linear) {
	t.Helper()
	box, ok := tt.Find("600519 chart")
	if !ok {
		t.Fatal("chart missing")
	}
	panes := kline.Split(box)
	vp := kline.Viewport{BarWidth: kline.DefaultBarWidth}
	vp.FitRight(d.chart.Bars(), float64(panes.Price.W))
	cs := make([]kline.Candle, d.chart.Bars())
	for i := range cs {
		cs[i], ok = d.chart.Bar("600519", i)
		if !ok {
			t.Fatalf("bar %d missing", i)
		}
	}
	return panes, vp, kline.MainScale(cs, nil, vp, float64(panes.Price.W))
}

func pixelsIn(img *image.RGBA, r ui.Rect, c ui.Color) int {
	count := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			p := img.RGBAAt(x, y)
			if math.Abs(float64(p.R)-float64(c.R)) < 12 && math.Abs(float64(p.G)-float64(c.G)) < 12 && math.Abs(float64(p.B)-float64(c.B)) < 12 {
				count++
			}
		}
	}
	return count
}

func TestTradesOpeningStackAndPeriodRemap(t *testing.T) {
	d, tt := tradesTester(t)
	if !tt.HasText("TRADE MARKERS") {
		t.Fatal("toolbar missing")
	}
	for _, period := range []string{"1h", "4h", "1h"} {
		if err := tt.Click(period); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if d.err != nil || d.chart.Period().String() != period {
			t.Fatalf("period: %s %v", d.chart.Period(), d.err)
		}
		marks, err := d.chart.Markers("600519")
		if err != nil || !reflect.DeepEqual(marks, d.saved) {
			t.Fatalf("markers changed: %v", err)
		}
		panes, vp, scale := tradeGeometry(t, d, tt)
		first, last := d.chart.Visible()
		step, err := d.chart.Period().Duration()
		if err != nil {
			t.Fatal(err)
		}
		indices := make([]int, len(marks))
		for j, m := range marks {
			indices[j] = -1
			for i := first; i <= last; i++ {
				bar, ok := d.chart.Bar("600519", i)
				if !ok {
					t.Fatal("visible bar missing")
				}
				if !m.Time.Before(bar.Time) && m.Time.Before(bar.Time.Add(step)) {
					indices[j] = i
					break
				}
			}
			if indices[j] < 0 {
				t.Fatalf("marker %d not visible in opening %s window %d..%d", j, period, first, last)
			}
		}
		if indices[0] != indices[1] || indices[1] != indices[2] || indices[2] == indices[3] {
			t.Fatalf("buys must stack; sell must be separate: %v", indices)
		}
		for _, j := range []int{0, 3} {
			tt.Move(panes.Price.X+float32(vp.BarLeft(indices[j])+vp.BarWidth/2), panes.Price.Y+panes.Price.H/2)
			tt.Frame()
			readout := d.chart.Readout("600519")
			labels := []string{"B1 100", "B2 100", "B3 100"}
			if j == 3 {
				labels = []string{"S 300"}
			}
			for _, label := range labels {
				if !strings.Contains(readout, label) {
					t.Fatalf("%s mapping: %q lacks %s", period, readout, label)
				}
			}
		}
		// Count all three arrow triangles in the actual rendered opening viewport.
		bar, ok := d.chart.Bar("600519", indices[0])
		if !ok {
			t.Fatal("buy bar missing")
		}
		x := panes.Price.X + float32(vp.BarLeft(indices[0])+vp.BarWidth/2)
		lowY := chart.Y(panes.Price, scale.At(bar.Low))
		for stack := range 3 {
			region := ui.Rect{X: x - 4, Y: lowY + 3 + float32(stack)*11, W: 9, H: 9}
			if n := pixelsIn(tt.Image(), region, d.chart.Style(nil).Buy); n < 8 {
				t.Fatalf("%s stack %d arrow has only %d colored pixels", period, stack, n)
			}
		}
	}
}

func TestTradesClearRestoreAndStyleControls(t *testing.T) {
	d, tt := tradesTester(t)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if d.err != nil {
			t.Fatal(d.err)
		}
	}
	click("Clear")
	marks, err := d.chart.Markers("600519")
	if err != nil || len(marks) != 0 {
		t.Fatalf("clear: %v %v", marks, err)
	}
	click("4h")
	click("Restore")
	marks, err = d.chart.Markers("600519")
	if err != nil || !reflect.DeepEqual(marks, d.saved) {
		t.Fatalf("restore: %v %v", marks, err)
	}
	click("1h")
	panes, vp, scale := tradeGeometry(t, d, tt)
	index := d.chart.Bars() - 24
	up, ok := d.chart.Bar("600519", index)
	if !ok {
		t.Fatal("limit-up candle missing")
	}
	down, ok := d.chart.Bar("600519", index+1)
	if !ok {
		t.Fatal("limit-down candle missing")
	}
	prev, ok := d.chart.Bar("600519", index-1)
	if !ok {
		t.Fatal("previous candle missing")
	}
	style := d.chart.Style(nil)
	if style.CandleBodyColor(up, prev.Close, 0.10) != style.LimitUp || style.CandleBodyColor(down, up.Close, 0.10) != style.LimitDown {
		t.Fatal("fixture never triggers limit colors")
	}
	region := ui.Rect{X: panes.Price.X + float32(vp.BarLeft(index)), Y: chart.Y(panes.Price, scale.At(up.Close)) + 3,
		W: float32(vp.BarWidth), H: chart.Y(panes.Price, scale.At(up.Open)) - chart.Y(panes.Price, scale.At(up.Close)) - 6}
	solid := pixelsIn(tt.Image(), region, style.LimitUp)
	if solid < 30 {
		t.Fatalf("limit-up body invisible: %d pixels", solid)
	}
	click("Hollow")
	if !d.chart.Hollow() || pixelsIn(tt.Image(), region, style.LimitUp) >= solid {
		t.Fatal("Hollow did not change rendered candle")
	}
	click("Palette")
	alternate := d.chart.Style(nil)
	if alternate.Buy == style.Buy || alternate.LimitUp == style.LimitUp || !d.chart.Hollow() || d.chart.LimitPct() != 0.10 {
		t.Fatal("palette lost colors or live style flags")
	}
	click("Limit colors")
	if d.chart.LimitPct() != 0 || pixelsIn(tt.Image(), region, alternate.LimitUp) != 0 {
		t.Fatal("limit colors stayed active")
	}
	click("Limit colors")
	if d.chart.LimitPct() != 0.10 || pixelsIn(tt.Image(), region, alternate.LimitUp) < 10 {
		t.Fatal("limit colors did not restore")
	}
	click("Palette")
	if d.chart.Style(nil).Buy != style.Buy || !d.chart.Hollow() {
		t.Fatal("palette restore failed")
	}
	// Returned errors must be visible; invalid restoration must keep prior marks.
	d.saved[0].Side = kline.MarkerSide(99)
	if err := tt.Click("Restore"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if d.err == nil || !tt.HasText("unknown marker side") {
		t.Fatal("restore error was discarded")
	}
}

func TestTradeFixtureAggregationAndValidation(t *testing.T) {
	f := newTradeFeed()
	req := kline.BarRequest{Symbol: "600519", Kind: kline.KindInit, Period: kline.Period{Span: 4, Unit: kline.UnitHour}, Limit: 1000}
	bars, more, err := f.GetBars(req)
	if err != nil || more || len(bars) != len(f.hourly)/4 {
		t.Fatalf("aggregate: len=%d more=%v err=%v", len(bars), more, err)
	}
	for i, b := range bars {
		group := f.hourly[i*4 : i*4+4]
		want := group[0]
		want.Close = group[3].Close
		want.Volume = 0
		for _, c := range group {
			want.High = math.Max(want.High, c.High)
			want.Low = math.Min(want.Low, c.Low)
			want.Volume += c.Volume
		}
		if b != want {
			t.Fatalf("4h bar %d differs from its hourly OHLCV: %v / %v", i, b, want)
		}
	}
	bars[0].Close = -1
	if f.hourly[0].Close == -1 {
		t.Fatal("response aliases fixture")
	}
	for _, edit := range []func(*kline.BarRequest){
		func(r *kline.BarRequest) { r.Symbol = "unknown" },
		func(r *kline.BarRequest) { r.Period.Span = 3 },
		func(r *kline.BarRequest) { r.Kind = kline.KindBackward },
		func(r *kline.BarRequest) { r.Kind = kline.KindForward; r.Anchor = time.Time{} },
	} {
		r := req
		edit(&r)
		if _, _, err := f.GetBars(r); err == nil {
			t.Fatalf("invalid request accepted: %+v", r)
		}
	}
}
