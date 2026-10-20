package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func hitHLine(o Overlay, x, y, plotW, plotH float64, scale chart.Linear) (int, bool) {
	if len(o.Points) == 0 || plotH <= 0 || plotW <= 0 {
		return 0, false
	}
	if x < 0 || x > plotW {
		return 0, false
	}
	ly := PriceToLocalY(plotH, o.Points[0].Price, scale)
	if math.Abs(y-ly) <= HitRadius {
		return HitBody, true
	}
	return 0, false
}

func drawHLine(p *ui.Painter, plot ui.Rect, o Overlay, scale chart.Linear, style Style) {
	if len(o.Points) == 0 {
		return
	}
	y := plot.Y + float32(PriceToLocalY(float64(plot.H), o.Points[0].Price, scale))
	color, width := overlayStroke(o, style)
	p.Line(plot.X, y, plot.X+plot.W, y, width, color)
	if o.Selected {
		drawHandle(p, plot.X+10, y, color)
	}
}
