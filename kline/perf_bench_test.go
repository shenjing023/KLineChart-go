package kline

import (
	"errors"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestCompareScalesMatchesFullNormalize(t *testing.T) {
	primary := Generate(120, 3)
	compare := Generate(120, 4)
	ma := NewMA(5, 20)
	ma.Calc(primary)
	boll := NewBOLL(20, 2)
	boll.Calc(primary)
	vp := Viewport{BarWidth: 10, Offset: 40}
	const plotW = 300.0
	index, raw, base := CompareScales(primary, compare, []Indicator{ma, boll}, vp, plotW)
	norm := NormalizeToIndex(primary, base)
	if norm == nil {
		t.Fatal("base close should index")
	}
	first, last := vp.Visible(commonLen(primary, compare), plotW)
	baseClose := primary[base].Close
	var extra [][]float64
	if line := NormalizeClose(compare, base); line != nil {
		extra = append(extra, line)
	}
	for _, series := range ma.Series() {
		extra = append(extra, fullScale(series, baseClose))
	}
	for _, series := range boll.Series() {
		extra = append(extra, fullScale(series, baseClose))
	}
	lo, hi := PriceDomain(norm, first, last, extra...)
	want := chart.NewLinear(lo, hi).Nice(5)
	if index.Lo != want.Lo || index.Hi != want.Hi {
		t.Fatalf("index lo %v hi %v, want lo %v hi %v", index.Lo, index.Hi, want.Lo, want.Hi)
	}
	if math.Abs(raw.Lo-want.Lo*baseClose/100) > 1e-9 || math.Abs(raw.Hi-want.Hi*baseClose/100) > 1e-9 {
		t.Fatalf("raw %+v want lo %v hi %v", raw, want.Lo*baseClose/100, want.Hi*baseClose/100)
	}
}

func fullScale(series []float64, baseClose float64) []float64 {
	out := make([]float64, len(series))
	for i, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[i] = math.NaN()
			continue
		}
		out[i] = v / baseClose * 100
	}
	return out
}

// TestCompareScalesAllocationBudget fails if a pan recomputes the overlay
// scale by copying the whole series. The visible window is about 100 bars.
func TestCompareScalesAllocationBudget(t *testing.T) {
	primary := Generate(5000, 1)
	compare := Generate(5000, 2)
	ma := NewMA(5, 20)
	ma.Calc(primary)
	boll := NewBOLL(20, 2)
	boll.Calc(primary)
	vp := Viewport{BarWidth: DefaultBarWidth, Offset: 4800}
	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			CompareScales(primary, compare, []Indicator{ma, boll}, vp, 960)
		}
	})
	if result.AllocedBytesPerOp() > 48*1024 {
		t.Fatalf("CompareScales allocated %d bytes/op, want the visible window under 48KB", result.AllocedBytesPerOp())
	}
}

// overlayPanByteCeiling is BenchmarkOverlayPanFrames/8k after the frame
// stopped computing CompareScales a second time. The measured frame is
// about 84KB. One extra CompareScales is about 15KB and lands above this.
const overlayPanByteCeiling = 92 * 1024

// TestOverlayPanAllocationBudget fails when an overlay pan frame starts
// allocating like it did when paint and DrawOverlays each called
// CompareScales. The bench has no drawings; TestOverlayFrameSkipsItemCopy
// covers that copy.
func TestOverlayPanAllocationBudget(t *testing.T) {
	result := testing.Benchmark(func(b *testing.B) {
		benchPanFrames(b, 8000, LayoutOverlay)
	})
	got := result.AllocedBytesPerOp()
	if got > overlayPanByteCeiling {
		t.Fatalf("overlay pan allocated %d bytes/op, ceiling is %d", got, overlayPanByteCeiling)
	}
}

// TestOverlayFrameSkipsItemCopy fails when paint deep-copies overlay
// points. The drawings use a Draw func that allocates nothing, so the
// extra bytes are the copy.
func TestOverlayFrameSkipsItemCopy(t *testing.T) {
	const kind = "quietmark"
	err := RegisterOverlay(OverlaySpec{
		Kind:    kind,
		Anchors: 1,
		Draw:    func(*ui.Painter, ui.Rect, Overlay, Viewport, chart.Linear, []Candle, Style) {},
		Hit: func(Overlay, float64, float64, float64, float64, Viewport, chart.Linear, []Candle) (int, bool) {
			return 0, false
		},
	})
	if err != nil && !errors.Is(err, ErrDuplicateOverlay) {
		t.Fatal(err)
	}
	const n = 200
	plain := overlayFrameBytes(t, 0, kind)
	with := overlayFrameBytes(t, n, kind)
	// A point-slice copy of n drawings is tens of kilobytes. Drawing
	// them with an empty Draw func should stay near the plain frame.
	if with > plain+8*1024 {
		t.Fatalf("frame with %d drawings allocated %d bytes/op, plain frame %d", n, with, plain)
	}
}

func overlayFrameBytes(t *testing.T, drawings int, kind string) int64 {
	t.Helper()
	result := testing.Benchmark(func(b *testing.B) {
		c, tt := benchChart(b, 1000, LayoutOverlay)
		for i := 0; i < drawings; i++ {
			c.panes[0].overlays.Add(kind, Point{BarIndex: i % 50, Price: 100 + float64(i)})
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dx := 12.0
			if i%2 == 1 {
				dx = -12
			}
			c.vp.Pan(dx, c.plotW, c.Bars())
			tt.Frame()
		}
	})
	return result.AllocedBytesPerOp()
}

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

// benchBarCounts is the matrix of series lengths. Generate's seed fixes
// the walk, so a given length is the same bars on every run.
var benchBarCounts = []struct {
	name string
	n    int
}{
	{"1k", 1000},
	{"8k", 8000},
	{"20k", 20000},
}

// benchIndicatorNames matches the indicators the frame benches paint.
// Overlay skips KDJ, as the 8k overlay pan bench did.
func benchIndicatorNames(layout string) []string {
	if layout == LayoutOverlay {
		return []string{"MA", "MACD", "BOLL"}
	}
	return []string{"MA", "MACD", "BOLL", "KDJ"}
}

// BenchmarkCompareScales is the overlay price-scale work done on pan and
// zoom. It stays off the UI so allocation changes in the scale itself
// show up directly.
func BenchmarkCompareScales(b *testing.B) {
	for _, size := range benchBarCounts {
		b.Run(size.name, func(b *testing.B) {
			primary := benchSeries(size.n, 1)
			compare := benchSeries(size.n, 2)
			inds := benchIndicators(primary)
			vp := Viewport{BarWidth: DefaultBarWidth, Offset: float64(size.n) - 140}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				CompareScales(primary, compare, inds, vp, 960)
			}
		})
	}
}

// BenchmarkIndicatorCalc is a data-change cost, not a pan cost. Indicators
// keep their series until candles are replaced.
func BenchmarkIndicatorCalc(b *testing.B) {
	for _, size := range benchBarCounts {
		b.Run(size.name, func(b *testing.B) {
			cs := benchSeries(size.n, 1)
			inds := benchIndicators(cs)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, ind := range inds {
					ind.Calc(cs)
				}
			}
		})
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

// benchChart mounts n bars and paints one frame at 960×720 so plotW is
// the width later gestures use.
func benchChart(b *testing.B, n int, layout string) (*Chart, *ui.Tester) {
	b.Helper()
	c := NewChart(ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Layout:     layout,
		Indicators: benchIndicatorNames(layout),
	})
	mountBenchBars(b, c, n)
	tt := ui.NewTester(c.View, 960, 720)
	tt.Frame()
	if c.plotW < 2 || c.Bars() != n {
		b.Fatalf("plot %v bars %d", c.plotW, c.Bars())
	}
	return c, tt
}

// benchPanFrames paints one frame per pan. Direction alternates by 12 DIPs
// so the window stays on the same bars instead of walking into the clamp.
// A pan does not walk bars outside the plot.
func benchPanFrames(b *testing.B, n int, layout string) {
	c, tt := benchChart(b, n, layout)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dx := 12.0
		if i%2 == 1 {
			dx = -12
		}
		c.vp.Pan(dx, c.plotW, c.Bars())
		tt.Frame()
	}
}

// BenchmarkPanFrames is the stack layout: two price plots, volume, and
// the open sub-panes.
func BenchmarkPanFrames(b *testing.B) {
	for _, size := range benchBarCounts {
		b.Run("stack_"+size.name, func(b *testing.B) {
			benchPanFrames(b, size.n, LayoutStack)
		})
	}
}

// BenchmarkOverlayPanFrames is the same pan on the indexed two-symbol plot.
func BenchmarkOverlayPanFrames(b *testing.B) {
	for _, size := range benchBarCounts {
		b.Run(size.name, func(b *testing.B) {
			benchPanFrames(b, size.n, LayoutOverlay)
		})
	}
}

// BenchmarkZoomFrames changes bar width by one wheel notch and paints.
// The notch alternates so width stays inside the 3–28 DIP clamp.
func BenchmarkZoomFrames(b *testing.B) {
	for _, layout := range []struct {
		name, value string
	}{
		{"stack", LayoutStack},
		{"overlay", LayoutOverlay},
	} {
		for _, size := range benchBarCounts {
			b.Run(layout.name+"_"+size.name, func(b *testing.B) {
				c, tt := benchChart(b, size.n, layout.value)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dy := 40.0
					if i%2 == 1 {
						dy = -40
					}
					c.vp.Zoom(dy, c.plotW/2, c.plotW, c.Bars())
					tt.Frame()
				}
			})
		}
	}
}

// BenchmarkCrosshairFrames moves the pointer between two bars. Move
// settles the frame the pointer event paints and the follow-up frame
// the crosshair requests when the index changes.
func BenchmarkCrosshairFrames(b *testing.B) {
	for _, layout := range []struct {
		name, value, label string
	}{
		{"stack", LayoutStack, "ALPHA chart"},
		{"overlay", LayoutOverlay, "Compare chart"},
	} {
		for _, size := range benchBarCounts {
			b.Run(layout.name+"_"+size.name, func(b *testing.B) {
				_, tt := benchChart(b, size.n, layout.value)
				box, ok := tt.Find(layout.label)
				if !ok || box.W < 240 {
					b.Fatalf("plot %q: %+v", layout.label, box)
				}
				y := box.Y + 40
				x0, x1 := box.X+80, box.X+200
				tt.Move(x0, y)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					x := x1
					if i%2 == 1 {
						x = x0
					}
					tt.Move(x, y)
				}
			})
		}
	}
}

// BenchmarkLivePushFrames revises ALPHA's last bar through StubLiveSource
// and paints one frame. The open time stays the same, so the series length
// does not grow during the run.
func BenchmarkLivePushFrames(b *testing.B) {
	for _, layout := range []struct {
		name, value string
	}{
		{"stack", LayoutStack},
		{"overlay", LayoutOverlay},
	} {
		for _, size := range benchBarCounts {
			b.Run(layout.name+"_"+size.name, func(b *testing.B) {
				benchLivePushFrames(b, size.n, layout.value)
			})
		}
	}
}

func benchLivePushFrames(b *testing.B, n int, layout string) {
	period := Period{Span: 1, Unit: UnitHour}
	src := &StubLiveSource{}
	if err := src.SetBars("ALPHA", period, benchSeries(n, 1)); err != nil {
		b.Fatal(err)
	}
	if err := src.SetBars("BETA", period, benchSeries(n, 2)); err != nil {
		b.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     period,
		Layout:     layout,
		Indicators: benchIndicatorNames(layout),
	})
	mountBenchBars(b, c, n)
	tt := ui.NewTester(c.View, 960, 720)
	tt.Frame()
	if c.plotW < 2 || c.Bars() != n || len(c.panes[0].candles) == 0 {
		b.Fatalf("plot %v bars %d", c.plotW, c.Bars())
	}
	revised := c.panes[0].candles[len(c.panes[0].candles)-1]
	revised.Close = revised.Open + 0.35
	if revised.High < revised.Close {
		revised.High = revised.Close
	}
	if revised.Low > revised.Close {
		revised.Low = revised.Close
	}
	src.Push("ALPHA", period, revised)
	tt.Frame()
	last := c.panes[0].candles[len(c.panes[0].candles)-1]
	if c.Bars() != n || last.Close != revised.Close {
		b.Fatalf("live revise bars %d close %v, want %d and %v", c.Bars(), last.Close, n, revised.Close)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.Push("ALPHA", period, revised)
		tt.Frame()
	}
}

// markerPanByteSlack is how many extra bytes a pan frame may spend on
// trade markers after the first frame has warmed the text cache and the
// triangle path. Copying every marker each frame is far above this.
const markerPanByteSlack = 16 * 1024

// TestMarkerPanAllocationBudget fails when a pan with 500 markers
// allocates like a per-frame copy of the marker list. The plain frame
// is the same stack pan with no markers.
func TestMarkerPanAllocationBudget(t *testing.T) {
	plain := markerPanBytes(t, 8000, 0)
	with := markerPanBytes(t, 8000, 500)
	if with > plain+markerPanByteSlack {
		t.Fatalf("pan with 500 markers allocated %d bytes/op, plain frame %d, slack %d", with, plain, markerPanByteSlack)
	}
}

func markerPanBytes(t *testing.T, n, count int) int64 {
	t.Helper()
	result := testing.Benchmark(func(b *testing.B) {
		benchMarkerPan(b, n, count)
	})
	return result.AllocedBytesPerOp()
}

// BenchmarkMarkerPanFrames pans a stack of 8000 bars with 500 markers.
// The markers sit on every 16th bar, so the visible window draws a few
// of them. Labels are B and S, which the text cache keeps after the
// warmup frame.
func BenchmarkMarkerPanFrames(b *testing.B) {
	b.Run("8k", func(b *testing.B) {
		benchMarkerPan(b, 8000, 500)
	})
}

func benchMarkerPan(b *testing.B, n, count int) {
	c, tt := benchChart(b, n, LayoutStack)
	cs := c.panes[0].candles
	marks := make([]Marker, 0, count)
	if count > 0 && len(cs) > 1 {
		step := len(cs) / count
		if step < 1 {
			step = 1
		}
		for i := 0; i < count && i*step < len(cs)-1; i++ {
			side := SideBuy
			if i%2 == 1 {
				side = SideSell
			}
			marks = append(marks, Marker{Time: cs[i*step].Time, Side: side})
		}
	}
	if err := c.SetMarkers("ALPHA", marks); err != nil {
		b.Fatal(err)
	}
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dx := 12.0
		if i%2 == 1 {
			dx = -12
		}
		c.vp.Pan(dx, c.plotW, c.Bars())
		tt.Frame()
	}
}
