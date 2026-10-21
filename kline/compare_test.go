package kline

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNormalizeCloseIndexesBaseBarTo100(t *testing.T) {
	cs := []Candle{
		{Open: 9, High: 11, Low: 8, Close: 10},
		{Open: 11, High: 13, Low: 10, Close: 12},
		{Open: 9, High: 10, Low: 7, Close: 8},
	}
	got := NormalizeClose(cs, 0)
	want := []float64{100, 120, 80}
	if len(got) != len(want) {
		t.Fatalf("len %d", len(got))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("close[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	// The base can be a later bar: that bar's close is 100, and every
	// earlier close is scaled by the same divisor.
	got = NormalizeClose(cs, 1)
	if math.Abs(got[1]-100) > 1e-9 || math.Abs(got[0]-10.0/12.0*100) > 1e-9 {
		t.Fatalf("base 1: %v", got)
	}
}

func TestNormalizeToIndexScalesOHLCByBaseClose(t *testing.T) {
	cs := []Candle{
		{Open: 9, High: 11, Low: 8, Close: 10, Volume: 40},
		{Open: 11, High: 13, Low: 10, Close: 12, Volume: 50},
	}
	got := NormalizeToIndex(cs, 0)
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if math.Abs(got[0].Close-100) > 1e-9 || math.Abs(got[0].Open-90) > 1e-9 ||
		math.Abs(got[0].High-110) > 1e-9 || math.Abs(got[0].Low-80) > 1e-9 {
		t.Fatalf("bar 0 %+v", got[0])
	}
	if math.Abs(got[1].Close-120) > 1e-9 || math.Abs(got[1].High-130) > 1e-9 {
		t.Fatalf("bar 1 %+v", got[1])
	}
	if got[0].Volume != 40 || got[1].Volume != 50 {
		t.Fatalf("volume should stay raw: %+v %+v", got[0], got[1])
	}
	if NormalizeToIndex(cs, 5) != nil {
		t.Fatal("a base outside the series should not invent bars")
	}
	flat := []Candle{{Close: 0, Open: 1, High: 1, Low: 1}}
	if NormalizeToIndex(flat, 0) != nil || NormalizeClose(flat, 0) != nil {
		t.Fatal("a non-positive base close cannot be the 100 level")
	}
}

func TestIndexBaseIsFirstVisibleCommonClose(t *testing.T) {
	primary := []Candle{{Close: 0}, {Close: 10}, {Close: 11}, {Close: 12}}
	compare := []Candle{{Close: 8}, {Close: 9}, {Close: 0}, {Close: 7}}

	// Index 0 is not common: the primary close is not positive.
	// Index 1 is the first bar in the window where both closes are positive.
	if got := IndexBase(0, 3, primary, compare); got != 1 {
		t.Fatalf("window base %d, want 1", got)
	}
	// A window that starts later uses that later common bar, not bar 0
	// of the series.
	if got := IndexBase(3, 3, primary, compare); got != 3 {
		t.Fatalf("late window base %d, want 3", got)
	}
	// The visible window has no positive common close, so the base falls
	// back to the first such bar of the series.
	if got := IndexBase(2, 2, primary, compare); got != 1 {
		t.Fatalf("fallback base %d, want 1", got)
	}
}

func TestCompareScaleDomainFollowsBothSeries(t *testing.T) {
	primary := []Candle{
		{Open: 10, High: 10, Low: 10, Close: 10},
		{Open: 10, High: 11, Low: 9, Close: 10},
	}
	compare := []Candle{
		{Close: 20},
		{Close: 30},
	}
	scale, raw, base := CompareScales(primary, compare, nil, Viewport{Offset: 0, BarWidth: 10}, 20)
	if base != 0 {
		t.Fatalf("base %d", base)
	}
	// Primary stays near 100. Compare goes 100 → 150, and the domain
	// has to include that, with the usual padding.
	if scale.Hi < 150 || scale.Lo > 90 {
		t.Fatalf("index scale lo %v hi %v", scale.Lo, scale.Hi)
	}
	// A raw primary price and its index land on the same fraction, so
	// a drawing stored in raw price sits on the indexed candles.
	fracIndex := scale.At(100)
	fracRaw := raw.At(primary[0].Close)
	if math.Abs(float64(fracIndex-fracRaw)) > 1e-6 {
		t.Fatalf("index frac %v raw frac %v", fracIndex, fracRaw)
	}
	// Index 150 is primary raw 15: the primary base close is 10, and
	// drawings are stored in that raw price, not the compare series'.
	fracIndex = scale.At(150)
	fracRaw = raw.At(15)
	if math.Abs(float64(fracIndex-fracRaw)) > 1e-6 {
		t.Fatalf("index 150 vs primary raw 15: %v %v", fracIndex, fracRaw)
	}
}

func TestCompareFallbackRequiresBothSeries(t *testing.T) {
	primary := Generate(40, 1)
	compare := append([]Candle(nil), primary...)
	for i := range primary {
		primary[i].Open /= 10
		primary[i].High /= 10
		primary[i].Low /= 10
		primary[i].Close /= 10
		compare[i].Open, compare[i].High, compare[i].Low, compare[i].Close = 0, 0, 0, 0
	}
	vp := Viewport{BarWidth: 8}
	plotW := 400.0
	index, raw, base := CompareScales(primary, compare, nil, vp, plotW)
	want := MainScale(primary, nil, vp, plotW)
	if base != 0 {
		t.Fatalf("base %d", base)
	}
	if index.Lo != want.Lo || index.Hi != want.Hi || raw.Lo != want.Lo || raw.Hi != want.Hi {
		t.Fatalf("no common positive close indexed %+v raw %+v, want raw %+v", index, raw, want)
	}
	panes := Split(ui.Rect{W: 400, H: 300})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
			if indexed := PaintCompare(p, panes, primary, compare, nil, vp, Pointer{}, -1, DarkDefault()); indexed {
				t.Error("PaintCompare reported an index without a common base")
			}
		})
	}, 400, 300)
	tt.Frame()
	c := NewChart(ChartOptions{
		Source:     &fixedPairSource{primary: primary, compare: compare},
		Symbols:    []string{"A", "B"},
		Layout:     LayoutOverlay,
		Indicators: []string{},
	})
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
	c.layout = layoutOverlay
	c.plotW = plotW
	c.vp = vp
	if got := c.Readout("A"); strings.Contains(got, "idx ") {
		t.Fatalf("fallback readout still reports an index: %q", got)
	}
	if got := c.priceScale(0); got.Lo != want.Lo || got.Hi != want.Hi {
		t.Fatalf("hit-test scale %+v, want raw %+v", got, want)
	}
}

func TestCompareSingleSeriesKeepsDocumentedBehavior(t *testing.T) {
	primary := Generate(20, 2)
	for i := range primary {
		primary[i].Close = 10
	}
	vp := Viewport{BarWidth: 8}
	index, raw, base := CompareScales(primary, nil, nil, vp, 160)
	if base != 0 || index.At(100) != raw.At(primary[0].Close) {
		t.Fatalf("single-series overlay base %d index %+v raw %+v", base, index, raw)
	}
}

type fixedPairSource struct {
	primary, compare []Candle
}

func (s *fixedPairSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if req.Symbol == "A" {
		return s.primary, false, nil
	}
	return s.compare, false, nil
}
