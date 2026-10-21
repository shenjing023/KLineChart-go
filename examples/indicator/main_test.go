package main

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

func assertFullCalc(t *testing.T, got *hostBand, cs []kline.Candle) {
	t.Helper()
	want := &hostBand{window: got.window, name: got.name}
	want.Calc(cs)
	if len(got.mean) != len(want.mean) || len(got.values) != len(want.values) {
		t.Fatal("tail length differs from Calc")
	}
	for i := range want.mean {
		if got.values[i] != want.values[i] || (got.mean[i] != want.mean[i] && !(math.IsNaN(got.mean[i]) && math.IsNaN(want.mean[i]))) {
			t.Fatalf("bar %d tail=%v/%v Calc=%v/%v", i, got.values[i], got.mean[i], want.values[i], want.mean[i])
		}
		if got.ValueAt(i) != want.ValueAt(i) {
			t.Fatalf("readout differs at %d", i)
		}
	}
	for _, bounds := range [][2]int{{0, len(cs) - 1}, {len(cs) - 4, len(cs) - 1}, {len(cs) - 1, len(cs) - 1}, {-10, 3}, {0, -1}, {len(cs), len(cs) + 5}} {
		if a, b := got.Scale(bounds[0], bounds[1]), want.Scale(bounds[0], bounds[1]); a.Lo != b.Lo || a.Hi != b.Hi {
			t.Fatalf("scale %v tail=%v Calc=%v", bounds, a, b)
		}
	}
}

func TestHostBandAppendAndReviseMatchFullCalc(t *testing.T) {
	for _, window := range []int{1, 3} {
		t.Run(string(rune('0'+window)), func(t *testing.T) {
			cs := kline.Generate(48, 13)
			got := &hostBand{window: window}
			got.Calc(nil)
			for i := range cs {
				got.Append(cs[:i+1])
				assertFullCalc(t, got, cs[:i+1])
				// Revisions repeatedly replace the current maximum/minimum, then remove it.
				for _, delta := range []float64{600, -600, 0, 2} {
					prev := cs[i]
					cs[i].Close = cs[i].Open + delta
					got.Revise(cs[:i+1], prev)
					assertFullCalc(t, got, cs[:i+1])
				}
			}
			got.Revise(nil, kline.Candle{})
			assertFullCalc(t, got, nil)
		})
	}
}

func TestHostBandDrawUsesVisibleScale(t *testing.T) {
	h := &hostBand{mean: make([]float64, 30)}
	for i := range h.mean {
		h.mean[i] = 2 + float64(i%5)*0.4
	}
	h.mean[2] = 1000 // Off-screen values must not squash the line against the axis.
	vp := kline.Viewport{Offset: 20, BarWidth: 16}
	var rect ui.Rect
	render := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { rect = r; h.DrawSub(p, r, vp) })
		}, 160, 120)
	}
	tt := render()
	before := append([]byte(nil), tt.Image().Pix...)
	first, last := vp.Visible(len(h.mean), float64(rect.W))
	scale := h.Scale(first, last)
	// The actual stroke must cross the position advertised by the axis scale.
	x := int(vp.BarLeft(21) + vp.BarWidth/2)
	y := int(chart.Y(rect, scale.At(h.mean[21])))
	cyan := false
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			p := tt.Image().RGBAAt(x+dx, y+dy)
			if p.G > 100 && p.B > 150 && p.R < 40 {
				cyan = true
			}
		}
	}
	if !cyan {
		t.Fatal("DrawSub did not use SubPlot's visible scale")
	}
	h.mean[2] = -1000
	if after := render().Image().Pix; !bytes.Equal(before, after) {
		t.Fatal("off-screen extrema changed the visible drawing")
	}
	h.mean[2] = math.Inf(1)
	if s := h.Scale(0, 29); math.IsInf(s.Lo, 0) || math.IsInf(s.Hi, 0) {
		t.Fatal("non-finite scale")
	}
}

func TestIndicatorViewToggleTailAndLateRegistration(t *testing.T) {
	kline.Unregister("HOSTFAST")
	t.Cleanup(func() { kline.Unregister("HOSTFAST") })
	d, err := newIndicatorDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.chart.Close(); err != nil {
			t.Error(err)
		}
	})
	tt := ui.NewTester(indicatorView(d), 960, 760)
	if !tt.HasText("HOST SUB-PANE") || !d.chart.IndicatorOn("HOSTBAND") {
		t.Fatal("host band missing")
	}
	if d.chart.IndicatorEnabled("HOSTFAST") != nil {
		t.Fatal("fast registered before chart construction")
	}
	ma := d.chart.IndicatorEnabled("MA")
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
	checkReadout := func() {
		t.Helper()
		cs := make([]kline.Candle, d.chart.Bars())
		for i := range cs {
			var ok bool
			cs[i], ok = d.chart.Bar("600519", i)
			if !ok {
				t.Fatalf("missing bar %d", i)
			}
		}
		want := &hostBand{}
		want.Calc(cs)
		if got := d.chart.Readout("600519"); !strings.Contains(got, want.ValueAt(len(cs)-1)) {
			t.Fatalf("tail readout %q", got)
		}
	}
	checkReadout()
	n := d.chart.Bars()
	click("Append")
	if d.chart.Bars() != n+1 {
		t.Fatal("Append button did not append")
	}
	checkReadout()
	before, ok := d.chart.Bar("600519", n)
	if !ok {
		t.Fatal("appended bar missing")
	}
	click("Revise")
	after, ok := d.chart.Bar("600519", n)
	if !ok || before.Time != after.Time || before.Close == after.Close || d.chart.Bars() != n+1 {
		t.Fatal("Revise button did not revise the same bar")
	}
	checkReadout()
	click("HOSTBAND")
	if d.chart.IndicatorOn("HOSTBAND") || strings.Contains(d.chart.Readout("600519"), "HOSTBAND ") {
		t.Fatal("toggle did not disable")
	}
	click("Append")
	click("Revise")
	click("HOSTBAND")
	if !d.chart.IndicatorOn("HOSTBAND") {
		t.Fatal("toggle did not restore")
	}
	checkReadout() // Re-enabling recalculates bars changed while disabled.
	click("Register fast")
	if !d.chart.IndicatorOn("HOSTFAST") || d.chart.IndicatorEnabled("MA") != ma {
		t.Fatal("SyncIndicators lost the new spec or existing binding")
	}
	if !strings.Contains(d.chart.Readout("600519"), "HOSTFAST ") {
		t.Fatal("late indicator not calculated")
	}
	click("Register fast") // A second click also works without duplicate registration failure.
	click("HOSTFAST")
	if d.chart.IndicatorOn("HOSTFAST") {
		t.Fatal("late indicator toggle did not disable")
	}
	if err := d.chart.SetSource(indicatorFeed(nil)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Append"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if d.err == nil || !tt.HasText("fixture has no last bar") {
		t.Fatal("Append error was discarded")
	}
}
