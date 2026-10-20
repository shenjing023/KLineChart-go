package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func hitSegment(o Overlay, x, y, plotW, plotH float64, vp Viewport, scale chart.Linear, cs []Candle) (int, bool) {
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
	case distToSegment(x, y, x0, y0, x1, y1) <= HitRadius:
		return HitBody, true
	default:
		return 0, false
	}
}

func drawSegment(p *ui.Painter, plot ui.Rect, o Overlay, vp Viewport, scale chart.Linear, cs []Candle, style Style) {
	if len(o.Points) < 2 {
		return
	}
	x0, y0 := anchorPixel(plot, o.Points[0], vp, scale, cs)
	x1, y1 := anchorPixel(plot, o.Points[1], vp, scale, cs)
	color, width := overlayStroke(o, style)
	p.Line(x0, y0, x1, y1, width, color)
	if o.Selected {
		drawHandle(p, x0, y0, color)
		drawHandle(p, x1, y1, color)
	}
}

func drawRubber(p *ui.Painter, plot ui.Rect, rubber Rubber, vp Viewport, scale chart.Linear, cs []Candle, style Style) {
	if rubber.Kind == OverlayFibonacci {
		drawFibRubber(p, plot, rubber, vp, scale, cs, style)
		return
	}
	x0, y0 := anchorPixel(plot, rubber.From, vp, scale, cs)
	x1 := plot.X + float32(rubber.X)
	y1 := plot.Y + float32(rubber.Y)
	color := style.Overlay
	if color.A == 0 {
		color = style.OverlayHot
	}
	p.Line(x0, y0, x1, y1, 1, color.Alpha(0.75))
}

func anchorPixel(plot ui.Rect, pt Point, vp Viewport, scale chart.Linear, cs []Candle) (x, y float32) {
	x = plot.X + float32(AnchorX(pt, vp, cs))
	y = plot.Y + float32(PriceToLocalY(float64(plot.H), pt.Price, scale))
	return x, y
}

func overlayStroke(o Overlay, style Style) (ui.Color, float32) {
	if o.Selected {
		return style.OverlayHot, 2.5
	}
	return style.Overlay, 1.5
}

func drawHandle(p *ui.Painter, x, y float32, color ui.Color) {
	const s float32 = 7
	p.Fill(ui.Rect{X: x - s/2, Y: y - s/2, W: s, H: s}, color, 1)
}
