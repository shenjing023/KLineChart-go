package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func benchSeries(n int, seed int64) []Candle {
	return Generate(n, seed)
}

func benchIndicators(cs []Candle) []Indicator {
	ma := NewMA(5, 20)
	ma.Calc(cs)
	ma.SetColors([]ui.Color{ui.RGB(80, 160, 255), ui.RGB(255, 180, 40)})
	boll := NewBOLL(20, 2)
	boll.Calc(cs)
	macd := NewMACD(12, 26, 9)
	macd.Calc(cs)
	kdj := NewKDJ(9, 3, 3)
	kdj.Calc(cs)
	return []Indicator{ma, boll, macd, kdj}
}

// BenchmarkCompareScales is the overlay price-scale work done on pan and
// zoom. It stays off the UI so allocation changes in the scale itself
// show up directly.
func BenchmarkCompareScales(b *testing.B) {
	const n = 8000
	primary := benchSeries(n, 1)
	compare := benchSeries(n, 2)
	inds := benchIndicators(primary)
	vp := Viewport{BarWidth: DefaultBarWidth, Offset: float64(n) - 140}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareScales(primary, compare, inds, vp, 960)
	}
}

// BenchmarkIndicatorCalc is a data-change cost, not a pan cost. Indicators
// keep their series until candles are replaced.
func BenchmarkIndicatorCalc(b *testing.B) {
	const n = 8000
	cs := benchSeries(n, 1)
	inds := benchIndicators(cs)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ind := range inds {
			ind.Calc(cs)
		}
	}
}

// mountBenchBars replaces the loaded window with n bars. Init requests are
// capped, so a host-sized series has to be installed on the panes directly.
func mountBenchBars(b *testing.B, c *Chart, n int) {
	b.Helper()
	for i := range c.panes {
		c.panes[i] = c.mount(c.panes[i], benchSeries(n, int64(i+1)), false)
	}
	c.vp.FitRight(c.Bars(), 900)
	c.fitted = true
	c.plotW = 900
}

// BenchmarkPanFrames paints a chart while the viewport moves. The series
// is larger than the plot, so a pan does not walk every bar.
func BenchmarkPanFrames(b *testing.B) {
	const n = 8000
	c := NewChart(ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Indicators: []string{"MA", "MACD", "BOLL", "KDJ"},
	})
	mountBenchBars(b, c, n)
	tt := ui.NewTester(c.View, 960, 720)
	tt.Frame()
	if c.plotW < 2 || c.Bars() != n {
		b.Fatalf("plot %v bars %d", c.plotW, c.Bars())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.vp.Pan(12, c.plotW, c.Bars())
		tt.Frame()
	}
}

// BenchmarkOverlayPanFrames is the same pan on the indexed two-symbol plot.
func BenchmarkOverlayPanFrames(b *testing.B) {
	const n = 8000
	c := NewChart(ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Layout:     LayoutOverlay,
		Indicators: []string{"MA", "MACD", "BOLL"},
	})
	mountBenchBars(b, c, n)
	tt := ui.NewTester(c.View, 960, 720)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.vp.Pan(12, c.plotW, c.Bars())
		tt.Frame()
	}
}
