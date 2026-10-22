package kline

import (
	"math"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

const (
	// bodyFrac is the candle body width as a fraction of BarWidth.
	// The rest is the gap between neighboring bodies.
	bodyFrac = 0.68
)

// MainScale is the price plot's Y domain: the visible bars, widened by
// any main-pane indicator lines, then rounded to tick marks. Hit testing
// and overlay drawing use this so a price maps to the pixel Paint used.
func MainScale(cs []Candle, inds []Indicator, vp Viewport, plotW float64) chart.Linear {
	var extra [][]float64
	first, last := vp.Visible(len(cs), plotW)
	for _, ind := range inds {
		if ind == nil || ind.PaneKind() != PaneMain {
			continue
		}
		if lp, ok := ind.(LinePlot); ok {
			extra = append(extra, lp.Series()...)
		}
	}
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
// indicators overlay the price plot. MACD is drawn in panes.MACD and
// KDJ in panes.KDJ when those rects have a height. Volume stays the
// built-in VOL strip.
//
// barIndex draws the vertical crosshair on that bar even when ptr is
// outside this pane, so several panes can share one bar. -1 draws the
// vertical line from ptr when ptr is over the plot, and draws nothing
// when it is not. The horizontal line and the price tag follow ptr only.
// The vertical line runs through the price plot, the volume strip, and
// each open sub-pane.
func Paint(p *ui.Painter, panes Panes, cs []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, style Style) {
	drawPrice(p, panes, cs, inds, vp, ptr, barIndex, barPaint{style: style}, defaultLabels())
}

// drawPrice is Paint, and it returns the price scale it used. A plot
// smaller than 2 DIPs draws nothing and returns a zero scale.
func drawPrice(p *ui.Painter, panes Panes, cs []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, paint barPaint, labels labelFmt) chart.Linear {
	return drawPriceHook(p, panes, cs, inds, vp, ptr, barIndex, paint, labels, nil)
}

// drawPriceHook is drawPrice with a sub-pane painter. Chart passes its
// host hook; the exported Paint leaves it nil.
func drawPriceHook(p *ui.Painter, panes Panes, cs []Candle, inds []Indicator, vp Viewport, ptr Pointer, barIndex int, paint barPaint, labels labelFmt, subDraw func(*ui.Painter, ui.Rect, Indicator, int)) chart.Linear {
	price := panes.Price
	style := paint.style
	if price.W < 2 || price.H < 2 {
		return chart.Linear{}
	}
	p.Fill(ui.Rect{X: price.X, Y: price.Y, W: price.W + panes.PriceAxis.W, H: panes.Time.Y + panes.Time.H - price.Y}, style.Plot, 0)
	p.Fill(panes.Volume, style.VolumeBg, 0)

	applyIndicatorStyle(inds, panes, style)

	// chart.Linear maps a value to a fraction that grows upward.
	// chart.Y turns that fraction into a window Y, with 0 at the
	// bottom of the rect and 1 at the top. MainScale is the same
	// domain DrawOverlays uses, so a hit lands on the pixel that
	// was painted.
	yScale := MainScale(cs, inds, vp, float64(price.W))
	// The axis labels follow the chart's price precision. Without this,
	// a host that asks for whole numbers on a 0AMV panel still reads
	// 4200.00 on the axis while its own header shows 4200.
	yScale.Format = labels.price
	first, last := vp.Visible(len(cs), float64(price.W))
	volScale := volumeScale(cs, first, last, labels)
	layers := subLayers(panes, inds, first, last)

	drawGrid(p, panes, cs, vp, yScale, volScale, style, labels)
	p.Clip(price, 0, func() {
		drawCandles(p, price, cs, vp, yScale, first, last, paint)
		for _, ind := range inds {
			if ind == nil || ind.PaneKind() != PaneMain {
				continue
			}
			if lp, ok := ind.(LinePlot); ok && lp.Series() != nil {
				drawLinePlot(p, price, lp, vp, yScale)
				continue
			}
			ind.DrawMain(p, price, vp, yScale)
		}
	})
	p.Clip(panes.Volume, 0, func() {
		drawVolume(p, panes.Volume, cs, vp, volScale, first, last, paint)
	})
	for _, layer := range layers {
		drawSubAxis(p, layer.plot, layer.axis, layer.scale, style)
		ind := layer.ind
		rect := layer.plot
		p.Clip(rect, 0, func() {
			ind.DrawSub(p, rect, vp)
		})
		if subDraw != nil {
			hooked := rect
			index := barIndex
			if index < 0 {
				index = len(cs) - 1
			}
			p.Clip(hooked, 0, func() {
				subDraw(p, hooked, ind, index)
			})
		}
	}
	drawHighLow(p, panes, cs, vp, yScale, first, last, style, labels)
	drawLastPrice(p, panes, cs, vp, yScale, style, labels)
	drawCrosshair(p, panes, cs, vp, yScale, layers, ptr, barIndex, style, labels)
	return yScale
}

// drawLastPrice marks the newest close with a dashed line across the
// price plot and a filled tag on the axis, so the current price is
// readable without moving the pointer. The color follows that bar's own
// body unless Style.LastPrice overrides it, and ClearLastPrice hides it.
func drawLastPrice(p *ui.Painter, panes Panes, cs []Candle, vp Viewport, scale chart.Linear, style Style, labels labelFmt) {
	if len(cs) == 0 {
		return
	}
	price := panes.Price
	if price.W < 2 || price.H < 2 {
		return
	}
	last := cs[len(cs)-1]
	if math.IsNaN(last.Close) || math.IsInf(last.Close, 0) {
		return
	}
	y := chart.Y(price, scale.At(last.Close))
	if y < price.Y-1 || y > price.Y+price.H+1 {
		return
	}
	color := style.LastPrice
	if color == (ui.Color{}) {
		color = style.Up
		if last.Close < last.Open {
			color = style.Down
		}
	}
	drawDashedLine(p, price.X, price.X+price.W, y, color)
	drawTagColored(p, panes.PriceAxis.X+4, y, labels.price(last.Close), color, style)
}

// drawHighLow tags the highest high and the lowest low of the visible
// window. The tag sits at the bar that owns the extreme, with the arrow
// pointing back at the bar. ClearHighLow hides both.
func drawHighLow(p *ui.Painter, panes Panes, cs []Candle, vp Viewport, scale chart.Linear, first, last int, style Style, labels labelFmt) {
	if clearSet(style, ClearHighLow) {
		return
	}
	price := panes.Price
	if price.W < 2 || price.H < 2 || first > last {
		return
	}
	hiBar, loBar := -1, -1
	hi, lo := math.Inf(-1), math.Inf(1)
	for i := first; i <= last && i < len(cs); i++ {
		c := cs[i]
		if !math.IsNaN(c.High) && !math.IsInf(c.High, 0) && c.High > hi {
			hi, hiBar = c.High, i
		}
		if !math.IsNaN(c.Low) && !math.IsInf(c.Low, 0) && c.Low < lo {
			lo, loBar = c.Low, i
		}
	}
	color := style.HighLow
	if color == (ui.Color{}) {
		color = style.Muted
	}
	if hiBar >= 0 {
		x := price.X + float32(vp.BarLeft(hiBar)+vp.BarWidth/2)
		y := chart.Y(price, scale.At(hi))
		drawExtremeTag(p, x, y, labels.price(hi), color, true)
	}
	if loBar >= 0 && (loBar != hiBar || lo != hi) {
		x := price.X + float32(vp.BarLeft(loBar)+vp.BarWidth/2)
		y := chart.Y(price, scale.At(lo))
		drawExtremeTag(p, x, y, labels.price(lo), color, false)
	}
}

// drawExtremeTag writes the value just above a high or just below a low,
// with a small triangle pointing at the bar. It stays inside the plot.
func drawExtremeTag(p *ui.Painter, x, y float32, text string, color ui.Color, above bool) {
	span := ui.Span{Text: text, Size: 10, Color: color}
	w, h := p.MeasureText(0, span)
	tx := x - w/2
	ty := y - h - 4
	if above {
		ty = y - h - 4
	} else {
		ty = y + 4
	}
	p.RichText(tx, ty, 0, span)
}

// drawDashedLine draws a horizontal dashed line by stepping short
// segments. Painter has no dash pattern for a line, only for a rect.
func drawDashedLine(p *ui.Painter, x0, y, x1 float32, color ui.Color) {
	const dash, gap = 4, 3
	for x := x0; x < x1; x += dash + gap {
		end := x + dash
		if end > x1 {
			end = x1
		}
		p.Line(x, y, end, y, 1, color)
	}
}

// drawTagColored is drawTag with an explicit fill color, used by the last
// price mark so the tag matches the line.
func drawTagColored(p *ui.Painter, x, y float32, text string, fill ui.Color, style Style) {
	span := ui.Span{Text: text, Size: 11, Color: style.LabelText}
	w, h := p.MeasureText(0, span)
	padX, padY := float32(4), float32(2)
	box := ui.Rect{X: x - w/2 - padX, Y: y - h/2 - padY, W: w + padX*2, H: h + padY*2}
	p.Fill(box, fill, 3)
	p.RichText(box.X+padX, box.Y+padY, 0, span)
}

// clearSet reports whether name was listed in Style.Clear, which forces
// that field transparent even when it holds a color.
func clearSet(s Style, name ColorClear) bool { return s.Clear&name != 0 }

// subLayer is one open sub-pane and the scale its axis uses.
type subLayer struct {
	ind   Indicator
	plot  ui.Rect
	axis  ui.Rect
	scale chart.Linear
}

// subLayers pairs each open sub-pane with the rect subBands assigned it.
// The band order is the same one layoutPanes used, so a band that has a
// rect is the band this function paints.
func subLayers(panes Panes, inds []Indicator, first, last int) []subLayer {
	var out []subLayer
	extraUsed := 0
	for i, band := range subBands(inds) {
		if band == bandNone {
			continue
		}
		var plot, axis ui.Rect
		switch band {
		case bandMACD:
			plot, axis = panes.MACD, panes.MACDAxis
		case bandKDJ:
			plot, axis = panes.KDJ, panes.KDJAxis
		default:
			plot, axis = extraSubRect(panes, extraUsed)
			extraUsed++
		}
		if plot.H <= 1 {
			continue
		}
		out = append(out, subLayer{ind: inds[i], plot: plot, axis: axis, scale: subScale(inds[i], first, last)})
	}
	return out
}

func subScale(ind Indicator, first, last int) chart.Linear {
	if s, ok := ind.(SubPlot); ok {
		scale := s.Scale(first, last)
		if scale.Hi <= scale.Lo {
			scale.Hi = scale.Lo + 1
		}
		return scale
	}
	return chart.NewLinear(0, 1)
}

func drawLinePlot(p *ui.Painter, plot ui.Rect, lp LinePlot, vp Viewport, yScale chart.Linear) {
	if lp == nil {
		return
	}
	colors := lp.LineColors()
	lines := lp.Series()
	n := 0
	for _, line := range lines {
		if len(line) > n {
			n = len(line)
		}
	}
	first, last := vp.Visible(n, float64(plot.W))
	for i, line := range lines {
		if i >= len(colors) || colors[i] == (ui.Color{}) {
			continue
		}
		drawMA(p, plot, line, vp, yScale, first, last, colors[i])
	}
}

func extraSubRect(panes Panes, i int) (plot, axis ui.Rect) {
	if i < 0 || i >= len(panes.Extra) {
		return ui.Rect{}, ui.Rect{}
	}
	plot = panes.Extra[i]
	if i < len(panes.ExtraAxis) {
		axis = panes.ExtraAxis[i]
	}
	return plot, axis
}

// applyIndicatorStyle copies theme colors onto the indicators Paint
// knows how to draw. Sub-pane colors are applied only when that pane
// is open. Callers do not need the indicators back.
func applyIndicatorStyle(inds []Indicator, panes Panes, style Style) {
	extraUsed := 0
	for i, band := range subBands(inds) {
		ind := inds[i]
		switch v := ind.(type) {
		case *MA:
			v.SetColors([]ui.Color{style.MA5, style.MA20})
		case *BOLL:
			v.SetColors(style.BOLLMid, style.BOLLUp, style.BOLLLow)
		case *MACD:
			plot := panes.MACD
			if band != bandMACD {
				plot, _ = extraSubRect(panes, extraUsed)
				extraUsed++
			}
			if plot.H > 1 {
				v.SetColors(style.MACDDIF, style.MACDDEA, style.Up, style.Down, style.MACDZero)
			}
		case *KDJ:
			plot := panes.KDJ
			if band != bandKDJ {
				plot, _ = extraSubRect(panes, extraUsed)
				extraUsed++
			}
			if plot.H > 1 {
				v.SetColors(style.KDJK, style.KDJD, style.KDJJ)
			}
		default:
			if band == bandExtra {
				extraUsed++
			}
		}
	}
}

// volumeScale is the volume strip's Y domain over the visible bars,
// rounded to ticks and carrying the host's volume formatter. It is one
// function so the axis labels and the assertions about them read the
// same scale.
func volumeScale(cs []Candle, first, last int, labels labelFmt) chart.Linear {
	scale := chart.NewLinear(0, VolumeMax(cs, first, last)).Nice(3)
	if scale.Lo < 0 {
		scale.Lo = 0
	}
	if scale.Hi <= scale.Lo {
		scale.Hi = scale.Lo + 1
	}
	if labels.volume != nil {
		scale.Format = labels.volume
	}
	return scale
}

// labelNear reports whether y is within a label's height of a tick the
// price axis already prints in the same column. The axis is shared by
// every band, so a label from one band can land on another's.
func labelNear(p *ui.Painter, y float32, price ui.Rect, scale chart.Linear) bool {
	_, h := p.MeasureText(0, ui.Span{Text: "0", Size: 11})
	for _, tick := range scale.Ticks(5) {
		if tick.At < -0.01 || tick.At > 1.01 {
			continue
		}
		if d := chart.Y(price, tick.At) - y; d < h && d > -h {
			return true
		}
	}
	return false
}

func drawGrid(p *ui.Painter, panes Panes, cs []Candle, vp Viewport, yScale, volScale chart.Linear, style Style, labels labelFmt) {
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
			// The volume axis label is dropped when it would land on a
			// price tick: the price plot's lowest label sits at the very
			// bottom of its band, a few DIPs above the volume strip's
			// top, and two numbers printed there read as one. Losing the
			// volume maximum is the smaller cost.
			if !labelNear(p, y, price, yScale) {
				drawLabel(p, panes.VolumeAxis.X+6, y, tick.Label, style.Muted, ui.Start)
			}
		}
	}
	for _, tick := range timeTicks(cs, vp, price.W, labels) {
		position, ok := timeBarPosition(cs, time.UnixMilli(int64(tick.Value)))
		if !ok {
			continue
		}
		x := price.X + float32((position-vp.Offset)*vp.BarWidth)
		if x < price.X || x > price.X+price.W {
			continue
		}
		p.Line(x, price.Y, x, bandsBottom(panes), 1, style.Grid)
		if panes.Time.H > 0 {
			drawLabel(p, x, panes.Time.Y+panes.Time.H/2, tick.Label, style.Muted, ui.Center)
		}
	}
	bands := []ui.Rect{panes.Price, panes.Volume}
	if panes.MACD.H > 0 {
		bands = append(bands, panes.MACD)
	}
	if panes.KDJ.H > 0 {
		bands = append(bands, panes.KDJ)
	}
	for _, band := range panes.Extra {
		if band.H > 0 {
			bands = append(bands, band)
		}
	}
	sort.Slice(bands, func(i, j int) bool { return bands[i].Y < bands[j].Y })
	for i := 1; i < len(bands); i++ {
		if bands[i].Y <= bands[i-1].Y+bands[i-1].H {
			continue
		}
		y := bands[i].Y - float32(paneGap)/2
		p.Line(price.X, y, price.X+price.W, y, 1, style.Grid)
	}
}

// timeTicks builds the six round times the axis may label. The ticks are
// chosen in the chart's display zone, while their Value stays an
// absolute instant: the candle axis maps that instant to X.
func timeTicks(cs []Candle, vp Viewport, plotW float32, labels labelFmt) []chart.Tick {
	if len(cs) == 0 || vp.BarWidth <= 0 || plotW <= 0 {
		return nil
	}
	left := TimeAt(cs, vp.Offset)
	right := TimeAt(cs, vp.Offset+float64(plotW)/vp.BarWidth)
	loc := labels.loc
	if loc == nil {
		loc = time.UTC
	}
	// Ticks are chosen and labeled in the display zone. Their Value stays
	// the absolute instant, which is what timeBarPosition maps to X.
	scale := chart.NewTime(left.In(loc), right.In(loc))
	if labels.format != nil || labels.tick != nil {
		// A host formatter changes only the text. The tick interval and
		// therefore every x position stay the same, so a masked date
		// cannot move the grid.
		scale.Format = func(t time.Time) string { return labels.at(t, TickAxis, "") }
	}
	return scale.Ticks(6)
}

// timeBarPosition maps t onto the same fractional bar axis as candles.
// Each interval between neighboring opens is stretched to one bar,
// while a tick outside the series extrapolates with the nearest pair.
// This keeps a calendar gap from shifting every later tick, and it
// still accepts ticks such as 09:30 between off-grid candles.
func timeBarPosition(cs []Candle, t time.Time) (float64, bool) {
	if len(cs) == 0 {
		return 0, false
	}
	if len(cs) == 1 || !t.After(cs[0].Time) {
		if len(cs) == 1 {
			return 0, t.Equal(cs[0].Time)
		}
		gap := cs[1].Time.Sub(cs[0].Time)
		if gap <= 0 {
			return 0, t.Equal(cs[0].Time)
		}
		return float64(t.Sub(cs[0].Time)) / float64(gap), true
	}
	last := len(cs) - 1
	if !t.Before(cs[last].Time) {
		gap := cs[last].Time.Sub(cs[last-1].Time)
		if gap <= 0 {
			return float64(last), t.Equal(cs[last].Time)
		}
		return float64(last) + float64(t.Sub(cs[last].Time))/float64(gap), true
	}
	i := sort.Search(len(cs), func(i int) bool { return !cs[i].Time.Before(t) }) - 1
	if i < 0 {
		i = 0
	}
	if i >= last {
		i = last - 1
	}
	gap := cs[i+1].Time.Sub(cs[i].Time)
	if gap <= 0 {
		return float64(i), t.Equal(cs[i].Time)
	}
	return float64(i) + float64(t.Sub(cs[i].Time))/float64(gap), true
}

// drawSubAxis labels a sub-pane's axis. Labels that would collide with a
// neighbor are dropped: a 28-DIP band printed three ticks stacks them on
// top of each other, which reads as one garbled number. The scale's own
// extremes are kept, and the middle one is what gives way.
func drawSubAxis(p *ui.Painter, plot, axis ui.Rect, scale chart.Linear, style Style) {
	if plot.H < 8 {
		return
	}
	_, h := p.MeasureText(0, ui.Span{Text: "0", Size: 11})
	var last float32
	have := false
	for _, tick := range scale.Ticks(3) {
		if tick.At < -0.01 || tick.At > 1.01 {
			continue
		}
		y := chart.Y(plot, tick.At)
		if have && y-last < h && y-last > -h {
			continue
		}
		drawLabel(p, axis.X+6, y, tick.Label, style.Muted, ui.Start)
		last, have = y, true
	}
}

func bandsBottom(panes Panes) float32 {
	bottom := panes.Volume.Y + panes.Volume.H
	consider := func(r ui.Rect) {
		if r.H > 0 && r.Y+r.H > bottom {
			bottom = r.Y + r.H
		}
	}
	consider(panes.MACD)
	consider(panes.KDJ)
	for _, r := range panes.Extra {
		consider(r)
	}
	return bottom
}

// barPaint carries what a bar's color depends on: the palette, and the
// host's per-bar rule when there is one. It exists so the two drawing
// functions take one argument instead of another copy of style plus a
// callback.
type barPaint struct {
	style Style
	// host decides a bar's color. Nil, or a zero answer, uses style.
	host func(i int, c, prev Candle) ui.Color
	// base is the overlay divisor. A zero means raw prices.
	base float64
}

// color is the candle and volume color for bar i. The host is asked
// with the original bars, never the overlay-scaled ones, so its rule can
// compare cents the way the exchange does.
func (b barPaint) color(cs []Candle, i int) ui.Color {
	if b.host != nil && i >= 0 && i < len(cs) {
		var prev Candle
		if i > 0 {
			prev = cs[i-1]
		}
		if got := b.host(i, cs[i], prev); got.A != 0 {
			return got
		}
	}
	c := cs[i]
	prev := math.NaN()
	if i > 0 {
		prev = cs[i-1].Close
	}
	if b.base > 0 {
		// The built-in rule compares the scaled close with the scaled
		// previous close. Both are divided by the same base, so the sign
		// of the change is unchanged; only the limit percent needs raw
		// prices, and the host callback is the path for that.
		c = scaleCandle(c, b.base)
		prev = scalePrice(prev, b.base)
	}
	return b.style.CandleBodyColor(c, prev, b.style.LimitPct)
}

func scaleCandle(c Candle, base float64) Candle {
	return Candle{
		Time:   c.Time,
		Open:   scalePrice(c.Open, base),
		High:   scalePrice(c.High, base),
		Low:    scalePrice(c.Low, base),
		Close:  scalePrice(c.Close, base),
		Volume: c.Volume,
	}
}

func drawCandles(p *ui.Painter, plot ui.Rect, cs []Candle, vp Viewport, yScale chart.Linear, first, last int, paint barPaint) {
	if first > last {
		return
	}
	bodyW := float32(vp.BarWidth * bodyFrac)
	if bodyW < 1 {
		bodyW = 1
	}
	style := paint.style
	for i := first; i <= last; i++ {
		c := cs[i]
		if paint.base > 0 {
			c = scaleCandle(c, paint.base)
		}
		up := c.Close >= c.Open
		color := paint.color(cs, i)
		cx := plot.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
		// High is a larger price, so its fraction is larger and its
		// window Y is smaller than Low's.
		yHigh := chart.Y(plot, yScale.At(c.High))
		yLow := chart.Y(plot, yScale.At(c.Low))
		yOpen := chart.Y(plot, yScale.At(c.Open))
		yClose := chart.Y(plot, yScale.At(c.Close))
		top := min(yOpen, yClose)
		bot := max(yOpen, yClose)
		h := bot - top
		flat := h < 1
		if flat {
			h = 1
			top = (yOpen+yClose)/2 - 0.5
			bot = top + 1
		}
		body := ui.Rect{X: cx - bodyW/2, Y: top, W: bodyW, H: h}
		// A-share hollow: a rising body is an outline and its interior
		// stays empty, so the wick is drawn in two pieces that stop at
		// the body's edges. A single line from high to low would show
		// through the empty body, which is not how the A-share candle
		// looks. A falling body, and a flat one, keep one line: the body
		// is filled, so a wick behind it cannot be seen anyway.
		if up && style.HollowUp && !flat {
			p.Line(cx, yHigh, cx, top, 1, color)
			p.Line(cx, bot, cx, yLow, 1, color)
			p.Stroke(body, color, 0, 1)
			continue
		}
		p.Line(cx, yHigh, cx, yLow, 1, color)
		if up && style.HollowUp {
			p.Stroke(body, color, 0, 1)
			continue
		}
		p.Fill(body, color, 0)
	}
}

func drawMA(p *ui.Painter, plot ui.Rect, series []float64, vp Viewport, yScale chart.Linear, first, last int, color ui.Color) {
	drawMAScaled(p, plot, series, vp, yScale, first, last, color, 0)
}

// drawMAScaled strokes series. baseClose > 0 indexes each value to 100
// at that close, which is the overlay scale. Zero draws the raw values.
func drawMAScaled(p *ui.Painter, plot ui.Rect, series []float64, vp Viewport, yScale chart.Linear, first, last int, color ui.Color, baseClose float64) {
	if len(series) == 0 || first > last {
		return
	}
	var path ui.Path
	drawing := false
	drawn := false
	for i := first; i <= last && i < len(series); i++ {
		v := series[i]
		if baseClose > 0 {
			v = scalePrice(v, baseClose)
		}
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

// drawIndexedClose strokes close/baseClose*100 for the visible bars.
func drawIndexedClose(p *ui.Painter, plot ui.Rect, cs []Candle, base int, vp Viewport, yScale chart.Linear, first, last int, color ui.Color) {
	if color == (ui.Color{}) || base < 0 || base >= len(cs) || first > last {
		return
	}
	baseClose := cs[base].Close
	if baseClose <= 0 || math.IsNaN(baseClose) {
		return
	}
	var path ui.Path
	drawing := false
	drawn := false
	for i := first; i <= last && i < len(cs); i++ {
		v := scalePrice(cs[i].Close, baseClose)
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

func drawVolume(p *ui.Painter, plot ui.Rect, cs []Candle, vp Viewport, volScale chart.Linear, first, last int, paint barPaint) {
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
		// The volume bar follows the candle's color, so a host rule that
		// paints a limit-up bar red also paints that day's volume red.
		color := paint.color(cs, i)
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

func drawCrosshair(p *ui.Painter, panes Panes, cs []Candle, vp Viewport, yScale chart.Linear, layers []subLayer, ptr Pointer, barIndex int, style Style, labels labelFmt) {
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
	// line continues through each open sub-pane.
	inPrice := ptr.On && ptr.Y >= price.Y && ptr.Y <= price.Y+price.H
	inVol := ptr.On && ptr.Y >= panes.Volume.Y && ptr.Y <= panes.Volume.Y+panes.Volume.H
	var hitLayer *subLayer
	for i := range layers {
		layer := &layers[i]
		if ptr.On && ptr.Y >= layer.plot.Y && ptr.Y <= layer.plot.Y+layer.plot.H {
			hitLayer = layer
			break
		}
	}
	if inPrice || inVol || hitLayer != nil {
		p.Line(price.X, ptr.Y, price.X+price.W, ptr.Y, 1, style.Cross)
	}
	if inPrice && price.H > 0 {
		// Fraction up the plot: 0 at the bottom, 1 at the top, which
		// is the space chart.Linear.Invert reads.
		frac := float32((price.Y+price.H)-ptr.Y) / price.H
		label := labels.price(yScale.Invert(frac))
		drawTag(p, panes.PriceAxis.X+4, ptr.Y, label, style)
	}
	if hitLayer != nil && hitLayer.plot.H > 0 {
		frac := float32((hitLayer.plot.Y+hitLayer.plot.H)-ptr.Y) / hitLayer.plot.H
		drawTag(p, hitLayer.axis.X+4, ptr.Y, labels.price(hitLayer.scale.Invert(frac)), style)
	}
	if panes.Time.H > 0 {
		drawTag(p, x, panes.Time.Y+panes.Time.H/2, labels.at(cs[i].Time, TickCrosshair, "01-02 15:04"), style)
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
