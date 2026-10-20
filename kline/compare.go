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
//
// When base has no positive primary close, the overlay cannot be
// indexed and PaintCompare draws Paint's plot instead. index and raw are
// then both MainScale, the scale that plot uses, so drawings and hit
// tests still land on its candles.
func CompareScales(primary, compare []Candle, inds []Indicator, vp Viewport, plotW float64) (index, raw chart.Linear, base int) {
	base, first, last := overlayBase(primary, compare, vp, plotW)
	if !indexable(primary, base) {
		index = MainScale(primary, inds, vp, plotW)
		return index, index, base
	}
	baseClose := primary[base].Close
	// Domain only reads the visible window. Copying that window keeps
	// PriceDomain's padding and avoids a full-series normalize on pan.
	if first > last {
		index = chart.NewLinear(0, 1).Nice(5)
		raw = rawScale(index, baseClose)
		return index, raw, base
	}
	norm := normalizeWindow(primary, baseClose, first, last)
	var extra [][]float64
	if len(compare) > 0 && base < len(compare) {
		compareClose := compare[base].Close
		if compareClose > 0 && !math.IsNaN(compareClose) {
			extra = append(extra, windowCloses(compare, compareClose, first, last))
		}
	}
	for _, ind := range inds {
		lp, ok := ind.(LinePlot)
		if !ok || ind == nil || ind.PaneKind() != PaneMain {
			continue
		}
		for _, line := range lp.Series() {
			extra = append(extra, windowScaled(line, baseClose, first, last))
		}
	}
	lo, hi := PriceDomain(norm, 0, len(norm)-1, extra...)
	index = chart.NewLinear(lo, hi).Nice(5)
	raw = rawScale(index, baseClose)
	return index, raw, base
}

// overlayBase is the bar the overlay indexes to 100 for the window vp
// shows, and that window. Every overlay reader uses it, so the header's
// index and the painted base agree.
func overlayBase(primary, compare []Candle, vp Viewport, plotW float64) (base, first, last int) {
	first, last = vp.Visible(commonLen(primary, compare), plotW)
	if len(compare) == 0 {
		return IndexBase(first, last, primary), first, last
	}
	return IndexBase(first, last, primary, compare), first, last
}

// indexable reports whether bar base of cs has a close that can be 100.
func indexable(cs []Candle, base int) bool {
	return base >= 0 && base < len(cs) && cs[base].Close > 0 && !math.IsNaN(cs[base].Close)
}

func rawScale(index chart.Linear, baseClose float64) chart.Linear {
	return chart.Linear{
		Lo: index.Lo * baseClose / 100,
		Hi: index.Hi * baseClose / 100,
	}
}

func normalizeWindow(cs []Candle, baseClose float64, first, last int) []Candle {
	out := make([]Candle, last-first+1)
	for i := first; i <= last; i++ {
		c := cs[i]
		c.Open = scalePrice(c.Open, baseClose)
		c.High = scalePrice(c.High, baseClose)
		c.Low = scalePrice(c.Low, baseClose)
		c.Close = scalePrice(c.Close, baseClose)
		out[i-first] = c
	}
	return out
}

func windowCloses(cs []Candle, baseClose float64, first, last int) []float64 {
	out := make([]float64, last-first+1)
	for i := first; i <= last; i++ {
		if i < 0 || i >= len(cs) {
			out[i-first] = math.NaN()
			continue
		}
		out[i-first] = scalePrice(cs[i].Close, baseClose)
	}
	return out
}

func windowScaled(series []float64, baseClose float64, first, last int) []float64 {
	out := make([]float64, last-first+1)
	for i := first; i <= last; i++ {
		if i < 0 || i >= len(series) {
			out[i-first] = math.NaN()
			continue
		}
		v := series[i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[i-first] = math.NaN()
			continue
		}
		out[i-first] = v / baseClose * 100
	}
	return out
}

func commonLen(primary, compare []Candle) int {
	if len(compare) == 0 || len(primary) < len(compare) {
		return len(primary)
	}
	return len(compare)
}

// PaintCompare draws primary as candles and compare as a close line on
// one price plot. Both are indexed to 100 at the first visible common
// bar. The volume strip and the MACD and KDJ sub-panes stay on the
// primary series, in raw units. It returns false when the base close cannot be
// indexed, and then draws the primary series with Paint instead.
func PaintCompare(p *ui.Painter, panes Panes, primary, compare []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, style Style) bool {
	_, ok := drawCompare(p, panes, primary, compare, inds, vp, ptr, barIndex, style)
	return ok
}

// drawCompare is PaintCompare, and it returns the scale drawings use.
// That is the raw scale on an indexed plot, and MainScale when the base
// close cannot be indexed. An empty primary draws nothing and still
// returns MainScale.
func drawCompare(p *ui.Painter, panes Panes, primary, compare []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, style Style) (chart.Linear, bool) {
	price := panes.Price
	if price.W < 2 || price.H < 2 {
		return chart.Linear{}, false
	}
	if len(primary) == 0 {
		// PaintCompare draws nothing. Drawings still use MainScale, which
		// is what CompareScales returns when the base cannot be indexed.
		return MainScale(primary, inds, vp, float64(price.W)), false
	}
	index, raw, base := CompareScales(primary, compare, inds, vp, float64(price.W))
	if !indexable(primary, base) {
		return drawPrice(p, panes, primary, inds, vp, ptr, barIndex, style), false
	}
	baseClose := primary[base].Close

	p.Fill(ui.Rect{X: price.X, Y: price.Y, W: price.W + panes.PriceAxis.W, H: panes.Time.Y + panes.Time.H - price.Y}, style.Plot, 0)
	p.Fill(panes.Volume, style.VolumeBg, 0)

	applyIndicatorStyle(inds, panes, style)

	first, last := vp.Visible(len(primary), float64(price.W))
	volScale := chart.NewLinear(0, VolumeMax(primary, first, last)).Nice(3)
	if volScale.Lo < 0 {
		volScale.Lo = 0
	}
	if volScale.Hi <= volScale.Lo {
		volScale.Hi = volScale.Lo + 1
	}

	layers := subLayers(panes, inds, first, last)

	drawGrid(p, panes, primary, vp, index, volScale, style)
	p.Clip(price, 0, func() {
		drawCandles(p, price, primary, vp, index, first, last, style, baseClose)
		if style.Compare != (ui.Color{}) {
			drawIndexedClose(p, price, compare, base, vp, index, first, last, style.Compare)
		}
		for _, ind := range inds {
			lp, ok := ind.(LinePlot)
			if !ok || ind == nil || ind.PaneKind() != PaneMain {
				continue
			}
			colors := lp.LineColors()
			for i, line := range lp.Series() {
				if i >= len(colors) || colors[i] == (ui.Color{}) {
					continue
				}
				drawMAScaled(p, price, line, vp, index, first, last, colors[i], baseClose)
			}
		}
	})
	p.Clip(panes.Volume, 0, func() {
		drawVolume(p, panes.Volume, primary, vp, volScale, first, last, style)
	})
	for _, layer := range layers {
		drawSubAxis(p, layer.plot, layer.axis, layer.scale, style)
		ind := layer.ind
		rect := layer.plot
		p.Clip(rect, 0, func() {
			ind.DrawSub(p, rect, vp)
		})
	}
	drawCrosshair(p, panes, primary, vp, index, layers, ptr, barIndex, style)
	// Drawings are stored in raw price. raw is the scale that puts them
	// on the indexed candles this function just painted.
	return raw, true
}
