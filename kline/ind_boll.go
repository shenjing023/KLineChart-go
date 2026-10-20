package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func init() {
	mustRegister(Spec{
		Name: "BOLL",
		Kind: PaneMain,
		New:  func() Indicator { return NewBOLL(20, 2) },
	})
}

// BOLL is Bollinger Bands on Close, drawn on the price plot.
// Period is the SMA window, usually 20. Mult is the band width in
// standard deviations, usually 2.
//
// Mid is the SMA of Close. Up and Low are Mid ± Mult × σ, where σ is
// the population standard deviation of Close over that same window
// (the sum of squared deviations divided by Period, then square root).
// That is the KLineChart divisor. The three lines start together once
// Period closes exist.
type BOLL struct {
	Period                      int
	Mult                        float64
	Mid, Up, Low                []float64
	MidColor, UpColor, LowColor ui.Color
	series                      [][]float64
}

// NewBOLL returns bands with the given window and multiplier. Draw
// colors start as DarkDefault; Paint replaces them from the active Style.
func NewBOLL(period int, mult float64) *BOLL {
	s := DarkDefault()
	return &BOLL{
		Period:   period,
		Mult:     mult,
		MidColor: s.BOLLMid,
		UpColor:  s.BOLLUp,
		LowColor: s.BOLLLow,
	}
}

func (b *BOLL) Name() string     { return "BOLL" }
func (b *BOLL) PaneKind() string { return PaneMain }

// Series and LineColors implement LinePlot. Series returns one header
// for the life of the indicator, updated in place by Calc, so a frame
// can read the three lines without allocating. It is nil before the
// first Calc.
func (b *BOLL) Series() [][]float64 {
	if b == nil {
		return nil
	}
	return b.series
}

func (b *BOLL) LineColors() []ui.Color {
	return []ui.Color{b.MidColor, b.UpColor, b.LowColor}
}

// SetColors replaces the middle, upper, and lower lines.
func (b *BOLL) SetColors(mid, up, low ui.Color) {
	if b == nil {
		return
	}
	b.MidColor, b.UpColor, b.LowColor = mid, up, low
}

func (b *BOLL) Calc(cs []Candle) {
	n := len(cs)
	b.Mid = nanSlice(n)
	b.Up = nanSlice(n)
	b.Low = nanSlice(n)
	period := b.Period
	if n == 0 || period < 1 {
		b.bindSeries()
		return
	}
	mid := SMA(cs, period)
	if mid == nil {
		b.bindSeries()
		return
	}
	b.Mid = mid
	for i := period - 1; i < n; i++ {
		m := mid[i]
		if math.IsNaN(m) {
			continue
		}
		var sum float64
		for j := i - period + 1; j <= i; j++ {
			d := cs[j].Close - m
			sum += d * d
		}
		std := math.Sqrt(sum / float64(period))
		b.Up[i] = m + b.Mult*std
		b.Low[i] = m - b.Mult*std
	}
	b.bindSeries()
}

func (b *BOLL) bindSeries() {
	if cap(b.series) < 3 {
		b.series = make([][]float64, 3)
	}
	b.series = b.series[:3]
	b.series[0] = b.Mid
	b.series[1] = b.Up
	b.series[2] = b.Low
}

func (b *BOLL) DrawMain(p *ui.Painter, priceRect ui.Rect, vp Viewport, y chart.Linear) {
	if b == nil || len(b.Mid) == 0 || priceRect.W < 2 {
		return
	}
	first, last := vp.Visible(len(b.Mid), float64(priceRect.W))
	drawMA(p, priceRect, b.Mid, vp, y, first, last, b.MidColor)
	drawMA(p, priceRect, b.Up, vp, y, first, last, b.UpColor)
	drawMA(p, priceRect, b.Low, vp, y, first, last, b.LowColor)
}

func (b *BOLL) DrawSub(*ui.Painter, ui.Rect, Viewport) {}

func (b *BOLL) ValueAt(i int) string {
	if b == nil || i < 0 || i >= len(b.Mid) || math.IsNaN(b.Mid[i]) {
		return ""
	}
	s := "MID " + FormatPrice(b.Mid[i])
	if i < len(b.Up) && !math.IsNaN(b.Up[i]) {
		s += "  UP " + FormatPrice(b.Up[i])
	}
	if i < len(b.Low) && !math.IsNaN(b.Low[i]) {
		s += "  LOW " + FormatPrice(b.Low[i])
	}
	return s
}
