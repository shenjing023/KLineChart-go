package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// NormalizeClose returns close[i] / close[base] * 100. Bar base is 100.
// Each series uses its own base close, so two symbols meet at 100 on
// that bar even when their prices differ. A base outside the series, or
// a base close that is not positive, returns nil.
func NormalizeClose(cs []Candle, base int) []float64 {
	if base < 0 || base >= len(cs) {
		return nil
	}
	baseClose := cs[base].Close
	if baseClose <= 0 || math.IsNaN(baseClose) {
		return nil
	}
	out := make([]float64, len(cs))
	for i, c := range cs {
		if math.IsNaN(c.Close) {
			out[i] = math.NaN()
			continue
		}
		out[i] = c.Close / baseClose * 100
	}
	return out
}

// NormalizeToIndex scales Open, High, Low, and Close by close[base], so
// that bar's close is 100. Volume and Time are copied: the volume strip
// stays in raw units. The same conditions as NormalizeClose return nil.
func NormalizeToIndex(cs []Candle, base int) []Candle {
	if base < 0 || base >= len(cs) {
		return nil
	}
	baseClose := cs[base].Close
	if baseClose <= 0 || math.IsNaN(baseClose) {
		return nil
	}
	out := make([]Candle, len(cs))
	for i, c := range cs {
		out[i] = c
		out[i].Open = scalePrice(c.Open, baseClose)
		out[i].High = scalePrice(c.High, baseClose)
		out[i].Low = scalePrice(c.Low, baseClose)
		out[i].Close = scalePrice(c.Close, baseClose)
	}
	return out
}

func scalePrice(v, baseClose float64) float64 {
	if math.IsNaN(v) {
		return math.NaN()
	}
	return v / baseClose * 100
}

// IndexBase is the first index in [first, last] where every series has a
// positive close. Overlay mode indexes both symbols to 100 there, which
// is the first visible bar they share. When that window has no such bar,
// the base is the first bar of the common prefix that does. An empty
// input returns 0.
func IndexBase(first, last int, series ...[]Candle) int {
	n := 0
	for _, s := range series {
		if len(s) == 0 {
			return 0
		}
		if n == 0 || len(s) < n {
			n = len(s)
		}
	}
	if n == 0 {
		return 0
	}
	if first < 0 {
		first = 0
	}
	if last >= n {
		last = n - 1
	}
	if first <= last {
		for i := first; i <= last; i++ {
			if commonPositiveClose(series, i) {
				return i
			}
		}
	}
	for i := 0; i < n; i++ {
		if commonPositiveClose(series, i) {
			return i
		}
	}
	return 0
}

func commonPositiveClose(series [][]Candle, i int) bool {
	if len(series) == 0 {
		return false
	}
	for _, s := range series {
		if i < 0 || i >= len(s) {
			return false
		}
		c := s[i].Close
		if c <= 0 || math.IsNaN(c) {
			return false
		}
	}
	return true
}

// CompareScales is the overlay price scale. index maps the normalized
// range (primary OHLC, the compare close, and primary moving averages).
// raw maps primary prices onto the same pixels, so a drawing stored in
// raw price lands on the indexed candles. base is the bar whose close
// is 100 on each series.
func CompareScales(primary, compare []Candle, inds []Indicator, vp Viewport, plotW float64) (index, raw chart.Linear, base int) {
	n := commonLen(primary, compare)
	first, last := vp.Visible(n, plotW)
	sets := [][]Candle{primary}
	if len(compare) > 0 {
		sets = append(sets, compare)
	}
	base = IndexBase(first, last, sets...)
	norm := NormalizeToIndex(primary, base)
	if norm == nil {
		index = chart.NewLinear(0, 1)
		return index, index, base
	}
	var extra [][]float64
	if line := NormalizeClose(compare, base); line != nil {
		extra = append(extra, line)
	}
	baseClose := primary[base].Close
	for _, ind := range inds {
		ma, ok := ind.(*MA)
		if !ok || ma == nil || ma.PaneKind() != PaneMain {
			continue
		}
		for _, line := range ma.Lines() {
			extra = append(extra, scaleLine(line, baseClose))
		}
	}
	lo, hi := PriceDomain(norm, first, last, extra...)
	index = chart.NewLinear(lo, hi).Nice(5)
	raw = chart.Linear{
		Lo: index.Lo * baseClose / 100,
		Hi: index.Hi * baseClose / 100,
	}
	return index, raw, base
}

func commonLen(primary, compare []Candle) int {
	if len(compare) == 0 || len(primary) < len(compare) {
		return len(primary)
	}
	return len(compare)
}

func scaleLine(series []float64, baseClose float64) []float64 {
	if baseClose <= 0 || math.IsNaN(baseClose) {
		return nil
	}
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

// PaintCompare draws primary as candles and compare as a close line on
// one price plot. Both are indexed to 100 at the first visible common
// bar. The volume strip and the MACD sub-pane stay on the primary
// series, in raw units. It returns false when the base close cannot be
// indexed, and then draws the primary series with Paint instead.
func PaintCompare(p *ui.Painter, panes Panes, primary, compare []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, style Style) bool {
	price := panes.Price
	if price.W < 2 || price.H < 2 || len(primary) == 0 {
		return false
	}
	index, _, base := CompareScales(primary, compare, inds, vp, float64(price.W))
	norm := NormalizeToIndex(primary, base)
	if norm == nil {
		Paint(p, panes, primary, inds, vp, ptr, barIndex, style)
		return false
	}

	p.Fill(ui.Rect{X: price.X, Y: price.Y, W: price.W + panes.PriceAxis.W, H: panes.Time.Y + panes.Time.H - price.Y}, style.Plot, 0)
	p.Fill(panes.Volume, style.VolumeBg, 0)

	var sub Indicator
	for _, ind := range inds {
		if ind == nil {
			continue
		}
		switch ind.PaneKind() {
		case PaneMain:
			if ma, ok := ind.(*MA); ok {
				ma.SetColors([]ui.Color{style.MA5, style.MA20})
			}
		case PaneSub:
			if sub == nil && panes.MACD.H > 1 {
				if macd, ok := ind.(*MACD); ok {
					macd.SetColors(style.MA5, style.MA20, style.Up, style.Down, style.Grid)
				}
				sub = ind
			}
		}
	}

	first, last := vp.Visible(len(norm), float64(price.W))
	volScale := chart.NewLinear(0, VolumeMax(primary, first, last)).Nice(3)
	if volScale.Lo < 0 {
		volScale.Lo = 0
	}
	if volScale.Hi <= volScale.Lo {
		volScale.Hi = volScale.Lo + 1
	}

	var macdScale chart.Linear
	hasMACD := false
	if sub != nil {
		if scaled, ok := sub.(interface {
			Scale(first, last int) chart.Linear
		}); ok {
			macdScale = scaled.Scale(first, last)
			hasMACD = true
		}
	}

	drawGrid(p, panes, norm, vp, index, volScale, style)
	baseClose := primary[base].Close
	p.Clip(price, 0, func() {
		drawCandles(p, price, norm, vp, index, first, last, style)
		if style.Compare != (ui.Color{}) {
			if line := NormalizeClose(compare, base); line != nil {
				drawMA(p, price, line, vp, index, first, last, style.Compare)
			}
		}
		for _, ind := range inds {
			ma, ok := ind.(*MA)
			if !ok || ma == nil {
				continue
			}
			for i, line := range ma.Lines() {
				color := ui.Color{}
				if i < len(ma.colors) {
					color = ma.colors[i]
				}
				if color == (ui.Color{}) {
					continue
				}
				drawMA(p, price, scaleLine(line, baseClose), vp, index, first, last, color)
			}
		}
	})
	p.Clip(panes.Volume, 0, func() {
		drawVolume(p, panes.Volume, primary, vp, volScale, first, last, style)
	})
	if sub != nil {
		if hasMACD {
			drawMACDAxis(p, panes, macdScale, style)
		}
		p.Clip(panes.MACD, 0, func() {
			sub.DrawSub(p, panes.MACD, vp)
		})
	}
	drawCrosshair(p, panes, norm, vp, index, macdScale, hasMACD, ptr, barIndex, style)
	return true
}
