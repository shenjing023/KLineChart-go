package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

const (
	// bodyFrac is the candle body width as a fraction of BarWidth.
	// The rest is the gap between neighboring bodies.
	bodyFrac = 0.68
)

// Style is the chart's colors. Up is a rising close (close >= open).
type Style struct {
	Up, Down       ui.Color
	MA5, MA20      ui.Color
	Grid, Cross    ui.Color
	Text, Muted    ui.Color
	Plot, VolumeBg ui.Color
	LabelFill      ui.Color
	LabelText      ui.Color
	// Overlay is a drawing that is not selected. OverlayHot is the
	// selected one, including its handles.
	Overlay, OverlayHot ui.Color
}

// StyleFrom builds colors that stay readable on the window theme.
// Rising bars are green and falling bars are red, matching KLineChart's
// default, on both light and dark themes.
func StyleFrom(t *ui.Theme) Style {
	if t == nil {
		t = ui.DarkTheme()
	}
	fill, labelText := t.Inverse, t.InverseText
	if fill == (ui.Color{}) {
		fill = t.Text
	}
	if labelText == (ui.Color{}) {
		labelText = t.Background
	}
	return Style{
		Up:         ui.RGB(38, 166, 154),
		Down:       ui.RGB(239, 83, 80),
		MA5:        t.Accent,
		MA20:       t.Warning,
		Grid:       t.Border,
		Cross:      t.TextMuted,
		Text:       t.Text,
		Muted:      t.TextMuted,
		Plot:       t.Background,
		VolumeBg:   t.Surface,
		LabelFill:  fill,
		LabelText:  labelText,
		Overlay:    ui.RGB(217, 70, 239),
		OverlayHot: ui.RGB(255, 214, 0),
	}
}

// MainScale is the price plot's Y domain: the visible bars, widened by
// any main-pane indicator lines, then rounded to tick marks. Hit testing
// and overlay drawing use this so a price maps to the pixel Paint used.
func MainScale(cs []Candle, inds []Indicator, vp Viewport, plotW float64) chart.Linear {
	var extra [][]float64
	for _, ind := range inds {
		if ind == nil || ind.PaneKind() != PaneMain {
			continue
		}
		if ma, ok := ind.(*MA); ok {
			extra = append(extra, ma.Lines()...)
		}
	}
	first, last := vp.Visible(len(cs), plotW)
	lo, hi := PriceDomain(cs, first, last, extra...)
	return chart.NewLinear(lo, hi).Nice(5)
}

// Pointer is the pointer in window DIPs while it is over the chart.
type Pointer struct {
	On   bool
	X, Y float32
}

// Paint draws the series in panes. vp is not modified. inds are the
// indicators mounted on this symbol; nil entries are skipped. Main
// indicators overlay the price plot. The first sub indicator is drawn
// in panes.MACD when that rect has a height. Volume stays the built-in
// VOL strip.
//
// barIndex draws the vertical crosshair on that bar even when ptr is
// outside this pane, so several panes can share one bar. -1 draws the
// vertical line from ptr when ptr is over the plot, and draws nothing
// when it is not. The horizontal line and the price tag follow ptr only.
// The vertical line runs through the price plot, the volume strip, and
// the MACD sub-pane.
func Paint(p *ui.Painter, panes Panes, cs []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, style Style) {
	price := panes.Price
	if price.W < 2 || price.H < 2 {
		return
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

	// chart.Linear maps a value to a fraction that grows upward.
	// chart.Y turns that fraction into a window Y, with 0 at the
	// bottom of the rect and 1 at the top. MainScale is the same
	// domain DrawOverlays uses, so a hit lands on the pixel that
	// was painted.
	yScale := MainScale(cs, inds, vp, float64(price.W))
	first, last := vp.Visible(len(cs), float64(price.W))
	volScale := chart.NewLinear(0, VolumeMax(cs, first, last)).Nice(3)
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

	drawGrid(p, panes, cs, vp, yScale, volScale, style)
	p.Clip(price, 0, func() {
		drawCandles(p, price, cs, vp, yScale, first, last, style)
		for _, ind := range inds {
			if ind != nil && ind.PaneKind() == PaneMain {
				ind.DrawMain(p, price, vp, yScale)
			}
		}
	})
	p.Clip(panes.Volume, 0, func() {
		drawVolume(p, panes.Volume, cs, vp, volScale, first, last, style)
	})
	if sub != nil {
		if hasMACD {
			drawMACDAxis(p, panes, macdScale, style)
		}
		p.Clip(panes.MACD, 0, func() {
			sub.DrawSub(p, panes.MACD, vp)
		})
	}
	drawCrosshair(p, panes, cs, vp, yScale, macdScale, hasMACD, ptr, barIndex, style)
}

func drawGrid(p *ui.Painter, panes Panes, cs []Candle, vp Viewport, yScale, volScale chart.Linear, style Style) {
	price := panes.Price
	for _, tick := range yScale.Ticks(5) {
		if tick.At < -0.01 || tick.At > 1.01 {
			continue
		}
		y := chart.Y(price, tick.At)
		p.Line(price.X, y, price.X+price.W, y, 1, style.Grid)
		drawLabel(p, panes.PriceAxis.X+6, y, tick.Label, style.Muted, ui.Start)
	}
	if panes.Volume.H > 8 {
		top := volScale.Ticks(2)
		if len(top) > 0 {
			tick := top[len(top)-1]
			y := chart.Y(panes.Volume, tick.At)
			drawLabel(p, panes.VolumeAxis.X+6, y, tick.Label, style.Muted, ui.Start)
		}
	}
	if len(cs) > 0 && vp.BarWidth > 0 {
		left := TimeAt(cs, vp.Offset)
		right := TimeAt(cs, vp.Offset+float64(price.W)/vp.BarWidth)
		for _, tick := range chart.NewTime(left, right).Ticks(6) {
			if tick.At < 0 || tick.At > 1 {
				continue
			}
			x := chart.X(price, tick.At)
			p.Line(x, price.Y, x, bandsBottom(panes), 1, style.Grid)
			if panes.Time.H > 0 {
				drawLabel(p, x, panes.Time.Y+panes.Time.H/2, tick.Label, style.Muted, ui.Center)
			}
		}
	}
	if panes.Volume.Y > price.Y+price.H {
		p.Line(price.X, panes.Volume.Y-float32(paneGap)/2, price.X+price.W, panes.Volume.Y-float32(paneGap)/2, 1, style.Grid)
	}
	if panes.MACD.H > 0 && panes.MACD.Y > panes.Volume.Y+panes.Volume.H {
		p.Line(price.X, panes.MACD.Y-float32(paneGap)/2, price.X+price.W, panes.MACD.Y-float32(paneGap)/2, 1, style.Grid)
	}
}

func drawMACDAxis(p *ui.Painter, panes Panes, scale chart.Linear, style Style) {
	if panes.MACD.H < 8 {
		return
	}
	for _, tick := range scale.Ticks(3) {
		if tick.At < -0.01 || tick.At > 1.01 {
			continue
		}
		y := chart.Y(panes.MACD, tick.At)
		drawLabel(p, panes.MACDAxis.X+6, y, tick.Label, style.Muted, ui.Start)
	}
}

func bandsBottom(panes Panes) float32 {
	if panes.MACD.H > 0 {
		return panes.MACD.Y + panes.MACD.H
	}
	return panes.Volume.Y + panes.Volume.H
}

func drawCandles(p *ui.Painter, plot ui.Rect, cs []Candle, vp Viewport, yScale chart.Linear, first, last int, style Style) {
	if first > last {
		return
	}
	bodyW := float32(vp.BarWidth * bodyFrac)
	if bodyW < 1 {
		bodyW = 1
	}
	for i := first; i <= last; i++ {
		c := cs[i]
		up := c.Close >= c.Open
		color := style.Down
		if up {
			color = style.Up
		}
		cx := plot.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
		// High is a larger price, so its fraction is larger and its
		// window Y is smaller than Low's.
		yHigh := chart.Y(plot, yScale.At(c.High))
		yLow := chart.Y(plot, yScale.At(c.Low))
		yOpen := chart.Y(plot, yScale.At(c.Open))
		yClose := chart.Y(plot, yScale.At(c.Close))
		p.Line(cx, yHigh, cx, yLow, 1, color)
		top := min(yOpen, yClose)
		bot := max(yOpen, yClose)
		h := bot - top
		if h < 1 {
			h = 1
			top = (yOpen+yClose)/2 - 0.5
		}
		p.Fill(ui.Rect{X: cx - bodyW/2, Y: top, W: bodyW, H: h}, color, 0)
	}
}

func drawMA(p *ui.Painter, plot ui.Rect, series []float64, vp Viewport, yScale chart.Linear, first, last int, color ui.Color) {
	if len(series) == 0 || first > last {
		return
	}
	var path ui.Path
	drawing := false
	drawn := false
	for i := first; i <= last && i < len(series); i++ {
		v := series[i]
		if math.IsNaN(v) {
			drawing = false
			continue
		}
		x := plot.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
		y := chart.Y(plot, yScale.At(v))
		drawn = true
		if !drawing {
			path.MoveTo(x, y)
			drawing = true
			continue
		}
		path.LineTo(x, y)
	}
	if drawn {
		p.StrokePath(&path, 1.5, color)
	}
}

func drawVolume(p *ui.Painter, plot ui.Rect, cs []Candle, vp Viewport, volScale chart.Linear, first, last int, style Style) {
	if first > last {
		return
	}
	bodyW := float32(vp.BarWidth * bodyFrac)
	if bodyW < 1 {
		bodyW = 1
	}
	floor := plot.Y + plot.H
	for i := first; i <= last; i++ {
		c := cs[i]
		color := style.Down
		if c.Close >= c.Open {
			color = style.Up
		}
		y := chart.Y(plot, volScale.At(c.Volume))
		h := floor - y
		if h < 1 {
			h = 1
			y = floor - 1
		}
		cx := plot.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
		p.Fill(ui.Rect{X: cx - bodyW/2, Y: y, W: bodyW, H: h}, color.Alpha(0.85), 0)
	}
}

func drawCrosshair(p *ui.Painter, panes Panes, cs []Candle, vp Viewport, yScale, macdScale chart.Linear, hasMACD bool, ptr Pointer, barIndex int, style Style) {
	if len(cs) == 0 {
		return
	}
	price := panes.Price
	i := barIndex
	if i < 0 && ptr.On {
		if hit, ok := vp.IndexAt(float64(ptr.X-price.X), len(cs)); ok {
			i = hit
		}
	}
	if i < 0 || i >= len(cs) {
		return
	}
	x := price.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
	if x < price.X || x > price.X+price.W {
		return
	}
	bottom := bandsBottom(panes)
	p.Line(x, price.Y, x, bottom, 1, style.Cross)

	// The horizontal line stays on the pane under the pointer. Other
	// panes only share the vertical line at this bar index. The vertical
	// line continues through the MACD sub-pane.
	inPrice := ptr.On && ptr.Y >= price.Y && ptr.Y <= price.Y+price.H
	inVol := ptr.On && ptr.Y >= panes.Volume.Y && ptr.Y <= panes.Volume.Y+panes.Volume.H
	inMACD := hasMACD && ptr.On && ptr.Y >= panes.MACD.Y && ptr.Y <= panes.MACD.Y+panes.MACD.H
	if inPrice || inVol || inMACD {
		p.Line(price.X, ptr.Y, price.X+price.W, ptr.Y, 1, style.Cross)
	}
	if inPrice && price.H > 0 {
		// Fraction up the plot: 0 at the bottom, 1 at the top, which
		// is the space chart.Linear.Invert reads.
		frac := float32((price.Y+price.H)-ptr.Y) / price.H
		label := FormatPrice(yScale.Invert(frac))
		drawTag(p, panes.PriceAxis.X+4, ptr.Y, label, style)
	}
	if inMACD && panes.MACD.H > 0 {
		frac := float32((panes.MACD.Y+panes.MACD.H)-ptr.Y) / panes.MACD.H
		drawTag(p, panes.MACDAxis.X+4, ptr.Y, FormatPrice(macdScale.Invert(frac)), style)
	}
	if panes.Time.H > 0 {
		drawTag(p, x, panes.Time.Y+panes.Time.H/2, cs[i].Time.UTC().Format("01-02 15:04"), style)
	}
}

// drawLabel draws text whose vertical center is y. align is the
// horizontal placement of x: ui.Start is the left edge, ui.Center the
// middle.
func drawLabel(p *ui.Painter, x, y float32, text string, color ui.Color, align ui.Align) {
	if text == "" {
		return
	}
	span := ui.Span{Text: text, Size: 11, Color: color}
	w, h := p.MeasureText(0, span)
	if align == ui.Center {
		x -= w / 2
	}
	p.RichText(x, y-h/2, 0, span)
}

func drawTag(p *ui.Painter, x, y float32, text string, style Style) {
	span := ui.Span{Text: text, Size: 11, Color: style.LabelText}
	w, h := p.MeasureText(0, span)
	padX, padY := float32(4), float32(2)
	box := ui.Rect{X: x - w/2 - padX, Y: y - h/2 - padY, W: w + padX*2, H: h + padY*2}
	p.Fill(box, style.LabelFill, 3)
	p.RichText(box.X+padX, box.Y+padY, 0, span)
}
