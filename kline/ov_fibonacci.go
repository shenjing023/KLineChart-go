package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// FibRatios are the standard retracement levels, from the first anchor
// through the second. Ratio 0 is the first click and 1 is the second.
var FibRatios = []float64{0, 0.236, 0.382, 0.5, 0.618, 0.786, 1}

// FibLevelPrice maps a ratio between two anchor prices. The anchors are
// not sorted into high and low first, so a downward draw still uses the
// click order.
func FibLevelPrice(anchor0, anchor1 float64, ratio float64) float64 {
	return anchor0 + ratio*(anchor1-anchor0)
}

func hitFibonacci(o Overlay, x, y, plotW, plotH float64, vp Viewport, scale chart.Linear, cs []Candle) (int, bool) {
	if len(o.Points) < 2 || plotH <= 0 || plotW <= 0 {
		return 0, false
	}
	x0 := AnchorX(o.Points[0], vp, cs)
	y0 := PriceToLocalY(plotH, o.Points[0].Price, scale)
	x1 := AnchorX(o.Points[1], vp, cs)
	y1 := PriceToLocalY(plotH, o.Points[1].Price, scale)
	d0 := math.Hypot(x-x0, y-y0)
	d1 := math.Hypot(x-x1, y-y1)
	switch {
	case d0 <= HitRadius && d0 <= d1:
		return 0, true
	case d1 <= HitRadius:
		return 1, true
	}
	left, right := fibSpanX(x0, x1)
	p0, p1 := o.Points[0].Price, o.Points[1].Price
	for _, ratio := range FibRatios {
		ly := PriceToLocalY(plotH, FibLevelPrice(p0, p1, ratio), scale)
		if hitFibLevel(x, y, left, right, ly) {
			return HitBody, true
		}
	}
	return 0, false
}

func hitFibLevel(x, y, left, right, levelY float64) bool {
	if left == right {
		return math.Hypot(x-left, y-levelY) <= HitRadius
	}
	return distToSegment(x, y, left, levelY, right, levelY) <= HitRadius
}

func drawFibonacci(p *ui.Painter, plot ui.Rect, o Overlay, vp Viewport, scale chart.Linear, cs []Candle, style Style) {
	if len(o.Points) < 2 {
		return
	}
	x0 := AnchorX(o.Points[0], vp, cs)
	x1 := AnchorX(o.Points[1], vp, cs)
	left, right := fibSpanX(x0, x1)
	p0, p1 := o.Points[0].Price, o.Points[1].Price
	color, width := overlayStroke(o, style)
	plotH := float64(plot.H)
	for _, ratio := range FibRatios {
		y := plot.Y + float32(PriceToLocalY(plotH, FibLevelPrice(p0, p1, ratio), scale))
		p.Line(plot.X+float32(left), y, plot.X+float32(right), y, width, color)
	}
	if o.Selected {
		hx0, hy0 := anchorPixel(plot, o.Points[0], vp, scale, cs)
		hx1, hy1 := anchorPixel(plot, o.Points[1], vp, scale, cs)
		drawHandle(p, hx0, hy0, color)
		drawHandle(p, hx1, hy1, color)
	}
}

func drawFibRubber(p *ui.Painter, plot ui.Rect, rubber Rubber, vp Viewport, scale chart.Linear, cs []Candle, style Style) {
	plotH := float64(plot.H)
	x0 := AnchorX(rubber.From, vp, cs)
	x1 := rubber.X
	left, right := fibSpanX(x0, x1)
	p1 := LocalYToPrice(plotH, rubber.Y, scale)
	p0 := rubber.From.Price
	color := style.Overlay
	if color.A == 0 {
		color = style.OverlayHot
	}
	color = color.Alpha(0.75)
	for _, ratio := range FibRatios {
		y := plot.Y + float32(PriceToLocalY(plotH, FibLevelPrice(p0, p1, ratio), scale))
		p.Line(plot.X+float32(left), y, plot.X+float32(right), y, 1, color)
	}
}

func fibSpanX(x0, x1 float64) (left, right float64) {
	if x0 <= x1 {
		return x0, x1
	}
	return x1, x0
}
