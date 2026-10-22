package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	// axisWidth fits a six-digit tick ("300000" measures about 36 DIP at
	// 11 px) plus the inset on each side, and no more. It is fixed rather
	// than measured per frame: a width that changed after the first paint
	// would move the plot under a pointer that had already been mapped,
	// so a click would land on the wrong bar.
	axisWidth = 56
	// axisPadding is the space on each side of a tick label inside the
	// axis column.
	axisPadding = 4
	timeHeight  = 22
	paneGap     = 8
	// volumeFrac is the volume band's share of the plot when both sub-panes
	// are off.
	volumeFrac = 0.22
	// volumeShare and macdShare split the plot when one sub-pane is open.
	// The price plot keeps the rest. Volume is the built-in VOL strip: it
	// stays a dedicated band, not an Indicator.
	volumeShare = 0.16
	macdShare   = 0.26
	// When MACD and KDJ are both open, each band takes a smaller share so
	// the price plot stays the largest. Order is price, volume, MACD, KDJ.
	volumeShareBoth = 0.14
	macdShareBoth   = 0.20
	kdjShareBoth    = 0.20
)

// PaneHeights is a host's override of the sub-pane bands, in DIPs. A
// zero field keeps the built-in share of the plot. The price plot takes
// whatever is left, and the layout still compresses the bands when the
// box is too short to honor them, so the value is a request rather than
// a guarantee.
type PaneHeights struct {
	// Sub is the height of each open sub-pane: MACD, KDJ, and any host
	// band. It is applied per band, so turning on a second sub-pane
	// takes another Sub from the price plot.
	Sub float64
	// Volume is the built-in volume strip's height.
	Volume float64
}

// Panes is one symbol's chart box split into the price plot, the built-in
// volume strip, optional MACD and KDJ sub-panes, and the axes. Coordinates
// are window DIPs, the same space a Draw callback receives. A sub-pane
// rect is empty when that band is off.
type Panes struct {
	Price, Volume, MACD, KDJ, Time ui.Rect
	PriceAxis                      ui.Rect
	VolumeAxis                     ui.Rect
	MACDAxis                       ui.Rect
	KDJAxis                        ui.Rect
	// Extra holds sub-panes past the built-in MACD and KDJ bands, in
	// indicator order. It is empty when only those built-ins are open,
	// so their rects stay on MACD and KDJ.
	Extra     []ui.Rect
	ExtraAxis []ui.Rect
}

// Split divides r into a price plot, a volume plot under it, a time
// axis along the bottom, and a price axis on the right.
func Split(r ui.Rect) Panes { return split(r, true, false, false) }

// SplitNoTime is the same layout without the time axis. A stack of
// panes uses it for every pane except the bottom one, which keeps the
// shared time labels.
func SplitNoTime(r ui.Rect) Panes { return split(r, false, false, false) }

// SplitStack divides r into price, the built-in volume strip, then the
// sub-panes that are on: MACD under volume, KDJ under MACD. A single
// sub-pane uses the MACD height either way. showTime keeps the time
// axis, which a stack draws only on the bottom symbol.
func SplitStack(r ui.Rect, showTime, showMACD, showKDJ bool) Panes {
	return split(r, showTime, showMACD, showKDJ)
}

func split(r ui.Rect, showTime, showMACD, showKDJ bool) Panes {
	return splitAxis(r, showTime, showMACD, showKDJ, axisWidth)
}

func splitAxis(r ui.Rect, showTime, showMACD, showKDJ bool, axis float32) Panes {
	// One sub-pane, whichever it is, keeps the MACD-only geometry. The
	// rect is moved onto KDJ when MACD itself is off.
	if showKDJ && !showMACD {
		panes := splitAxis(r, showTime, true, false, axis)
		panes.KDJ = panes.MACD
		panes.KDJAxis = panes.MACDAxis
		panes.MACD = ui.Rect{}
		panes.MACDAxis = ui.Rect{}
		return panes
	}
	if showMACD && showKDJ {
		return splitBoth(r, showTime, axis)
	}
	return splitOne(r, showTime, showMACD, axis)
}

func splitOne(r ui.Rect, showTime, showMACD bool, axis float32) Panes {
	axisW := axis
	timeH := float32(0)
	if showTime {
		timeH = timeHeight
	}
	gap := float32(paneGap)
	if r.W < axisW+40 {
		axisW = max(r.W*0.18, 1)
	}
	plotW := max(r.W-axisW, 1)
	bodyH := r.H - timeH
	if bodyH < gap+8 {
		bodyH = r.H
		timeH = 0
	}
	gaps := gap
	if showMACD {
		gaps = gap * 2
	}
	var volH, macdH float32
	if showMACD {
		inner := bodyH - gaps
		if inner < 1 {
			inner = 1
		}
		volH = float32(math.Round(float64(inner * volumeShare)))
		macdH = float32(math.Round(float64(inner * macdShare)))
		if volH < 16 && bodyH > 80 {
			volH = 16
		}
		if macdH < 28 && bodyH > 100 {
			macdH = 28
		}
	} else {
		volH = float32(math.Round(float64((bodyH - gap) * volumeFrac)))
		if volH < 20 && bodyH > 60 {
			volH = 20
		}
	}
	priceH := bodyH - gaps - volH - macdH
	if priceH < 1 {
		priceH = 1
		room := bodyH - gaps - priceH
		if room < 1 {
			room = 1
		}
		if showMACD {
			macdH = float32(math.Round(float64(room) * macdShare / (volumeShare + macdShare)))
			if macdH < 1 {
				macdH = 1
			}
			if macdH > room-1 {
				macdH = room - 1
			}
			volH = room - macdH
		} else {
			volH = max(room, 1)
		}
	}
	volY := r.Y + priceH + gap
	macdY := volY + volH
	timeY := macdY
	if showMACD {
		macdY = volY + volH + gap
		timeY = macdY + macdH
	}
	return fitPanes(r, Panes{
		Price:      ui.Rect{X: r.X, Y: r.Y, W: plotW, H: priceH},
		Volume:     ui.Rect{X: r.X, Y: volY, W: plotW, H: volH},
		MACD:       ui.Rect{X: r.X, Y: macdY, W: plotW, H: macdH},
		Time:       ui.Rect{X: r.X, Y: timeY, W: plotW, H: timeH},
		PriceAxis:  ui.Rect{X: r.X + plotW, Y: r.Y, W: axisW, H: priceH},
		VolumeAxis: ui.Rect{X: r.X + plotW, Y: volY, W: axisW, H: volH},
		MACDAxis:   ui.Rect{X: r.X + plotW, Y: macdY, W: axisW, H: macdH},
	})
}

// splitBoth is price, volume, MACD, then KDJ. Each sub-pane has its own
// Y axis. The time axis, when shown, sits under KDJ.
func splitBoth(r ui.Rect, showTime bool, axis float32) Panes {
	axisW := float32(axisWidth)
	timeH := float32(0)
	if showTime {
		timeH = timeHeight
	}
	gap := float32(paneGap)
	if r.W < axisW+40 {
		axisW = max(r.W*0.18, 1)
	}
	plotW := max(r.W-axisW, 1)
	bodyH := r.H - timeH
	if bodyH < gap+8 {
		bodyH = r.H
		timeH = 0
	}
	gaps := gap * 3
	inner := bodyH - gaps
	if inner < 1 {
		inner = 1
	}
	volH := float32(math.Round(float64(inner * volumeShareBoth)))
	macdH := float32(math.Round(float64(inner * macdShareBoth)))
	kdjH := float32(math.Round(float64(inner * kdjShareBoth)))
	if volH < 16 && bodyH > 120 {
		volH = 16
	}
	if macdH < 24 && bodyH > 160 {
		macdH = 24
	}
	if kdjH < 24 && bodyH > 160 {
		kdjH = 24
	}
	priceH := bodyH - gaps - volH - macdH - kdjH
	if priceH < 1 {
		priceH = 1
		room := bodyH - gaps - priceH
		if room < 3 {
			room = 3
		}
		sum := volumeShareBoth + macdShareBoth + kdjShareBoth
		volH = float32(math.Round(float64(room) * volumeShareBoth / sum))
		macdH = float32(math.Round(float64(room) * macdShareBoth / sum))
		if volH < 1 {
			volH = 1
		}
		if macdH < 1 {
			macdH = 1
		}
		if volH+macdH > room-1 {
			macdH = room - volH - 1
		}
		kdjH = room - volH - macdH
	}
	volY := r.Y + priceH + gap
	macdY := volY + volH + gap
	kdjY := macdY + macdH + gap
	timeY := kdjY + kdjH
	return fitPanes(r, Panes{
		Price:      ui.Rect{X: r.X, Y: r.Y, W: plotW, H: priceH},
		Volume:     ui.Rect{X: r.X, Y: volY, W: plotW, H: volH},
		MACD:       ui.Rect{X: r.X, Y: macdY, W: plotW, H: macdH},
		KDJ:        ui.Rect{X: r.X, Y: kdjY, W: plotW, H: kdjH},
		Time:       ui.Rect{X: r.X, Y: timeY, W: plotW, H: timeH},
		PriceAxis:  ui.Rect{X: r.X + plotW, Y: r.Y, W: axisW, H: priceH},
		VolumeAxis: ui.Rect{X: r.X + plotW, Y: volY, W: axisW, H: volH},
		MACDAxis:   ui.Rect{X: r.X + plotW, Y: macdY, W: axisW, H: macdH},
		KDJAxis:    ui.Rect{X: r.X + plotW, Y: kdjY, W: axisW, H: kdjH},
	})
}

// splitMany is price, volume, then n sub-panes of equal height. n > 2.
// The built-in one- and two-band layouts stay on SplitStack so their
// rects do not move.
func splitMany(r ui.Rect, showTime bool, n int) Panes {
	return splitManyAxis(r, showTime, n, axisWidth)
}

func splitManyAxis(r ui.Rect, showTime bool, n int, axis float32) Panes {
	if n < 1 {
		return SplitStack(r, showTime, false, false)
	}
	axisW := axis
	timeH := float32(0)
	if showTime {
		timeH = timeHeight
	}
	gap := float32(paneGap)
	if r.W < axisW+40 {
		axisW = max(r.W*0.18, 1)
	}
	plotW := max(r.W-axisW, 1)
	bodyH := r.H - timeH
	if bodyH < gap+8 {
		bodyH = r.H
		timeH = 0
	}
	gaps := gap * float32(n+1)
	inner := bodyH - gaps
	if inner < 1 {
		inner = 1
	}
	volFrac := 0.12
	subFrac := 0.16
	sum := volFrac + subFrac*float64(n)
	if sum > 0.72 {
		scale := 0.72 / sum
		volFrac *= scale
		subFrac *= scale
	}
	volH := float32(math.Round(float64(inner) * volFrac))
	subH := float32(math.Round(float64(inner) * subFrac))
	if volH < 12 && bodyH > 80 {
		volH = 12
	}
	if subH < 18 && bodyH > 100 {
		subH = 18
	}
	priceH := bodyH - gaps - volH - subH*float32(n)
	if priceH < 1 {
		priceH = 1
		room := bodyH - gaps - priceH
		if room < float32(n+1) {
			room = float32(n + 1)
		}
		volH = max(room*0.2, 1)
		subH = max((room-volH)/float32(n), 1)
		priceH = bodyH - gaps - volH - subH*float32(n)
		if priceH < 1 {
			priceH = 1
		}
	}
	volY := r.Y + priceH + gap
	extras := make([]ui.Rect, n)
	extraAxes := make([]ui.Rect, n)
	y := volY + volH + gap
	for i := 0; i < n; i++ {
		extras[i] = ui.Rect{X: r.X, Y: y, W: plotW, H: subH}
		extraAxes[i] = ui.Rect{X: r.X + plotW, Y: y, W: axisW, H: subH}
		y += subH + gap
	}
	return fitPanes(r, Panes{
		Price:      ui.Rect{X: r.X, Y: r.Y, W: plotW, H: priceH},
		Volume:     ui.Rect{X: r.X, Y: volY, W: plotW, H: volH},
		Extra:      extras,
		ExtraAxis:  extraAxes,
		Time:       ui.Rect{X: r.X, Y: y - gap, W: plotW, H: timeH},
		PriceAxis:  ui.Rect{X: r.X + plotW, Y: r.Y, W: axisW, H: priceH},
		VolumeAxis: ui.Rect{X: r.X + plotW, Y: volY, W: axisW, H: volH},
	})
}

// fitPanes keeps a collapsed layout inside its host rectangle. The
// normal shares already do this; the fallback used when every fixed
// minimum does not fit needs one more pass so a host never paints into
// a neighbouring element.
func fitPanes(r ui.Rect, panes Panes) Panes {
	for _, p := range []*ui.Rect{
		&panes.Price, &panes.Volume, &panes.MACD, &panes.KDJ, &panes.Time,
		&panes.PriceAxis, &panes.VolumeAxis, &panes.MACDAxis, &panes.KDJAxis,
	} {
		*p = fitRect(r, *p)
	}
	for i := range panes.Extra {
		panes.Extra[i] = fitRect(r, panes.Extra[i])
	}
	for i := range panes.ExtraAxis {
		panes.ExtraAxis[i] = fitRect(r, panes.ExtraAxis[i])
	}
	return panes
}

func fitRect(r, p ui.Rect) ui.Rect {
	if p.W <= 0 || p.H <= 0 {
		return p
	}
	if p.X < r.X {
		p.W -= r.X - p.X
		p.X = r.X
	}
	if p.Y < r.Y {
		p.H -= r.Y - p.Y
		p.Y = r.Y
	}
	if p.X+p.W > r.X+r.W {
		p.W = r.X + r.W - p.X
	}
	if p.Y+p.H > r.Y+r.H {
		p.H = r.Y + r.H - p.Y
	}
	if p.W < 0 {
		p.W = 0
	}
	if p.H < 0 {
		p.H = 0
	}
	return p
}
