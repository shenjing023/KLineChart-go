package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	axisWidth  = 72
	timeHeight = 22
	paneGap    = 8
	// volumeFrac is the volume band's share of the plot when MACD is off.
	volumeFrac = 0.22
	// volumeShare and macdShare split the plot when the MACD sub-pane is
	// open. The price plot keeps the rest. Volume is the built-in VOL
	// strip: it stays a dedicated band, not an Indicator.
	volumeShare = 0.16
	macdShare   = 0.26
)

// Panes is one symbol's chart box split into the price plot, the built-in
// volume strip, an optional MACD sub-pane, and the axes. Coordinates are
// window DIPs, the same space a Draw callback receives. MACD is empty
// when that sub-pane is off.
type Panes struct {
	Price, Volume, MACD, Time ui.Rect
	PriceAxis                 ui.Rect
	VolumeAxis                ui.Rect
	MACDAxis                  ui.Rect
}

// Split divides r into a price plot, a volume plot under it, a time
// axis along the bottom, and a price axis on the right.
func Split(r ui.Rect) Panes { return split(r, true, false) }

// SplitNoTime is the same layout without the time axis. A stack of
// panes uses it for every pane except the bottom one, which keeps the
// shared time labels.
func SplitNoTime(r ui.Rect) Panes { return split(r, false, false) }

// SplitStack divides r into price, the built-in volume strip, and, when
// showMACD is set, a MACD sub-pane under the volume. showTime keeps the
// time axis, which a stack draws only on the bottom symbol.
func SplitStack(r ui.Rect, showTime, showMACD bool) Panes {
	return split(r, showTime, showMACD)
}

func split(r ui.Rect, showTime, showMACD bool) Panes {
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
	return Panes{
		Price:      ui.Rect{X: r.X, Y: r.Y, W: plotW, H: priceH},
		Volume:     ui.Rect{X: r.X, Y: volY, W: plotW, H: volH},
		MACD:       ui.Rect{X: r.X, Y: macdY, W: plotW, H: macdH},
		Time:       ui.Rect{X: r.X, Y: timeY, W: plotW, H: timeH},
		PriceAxis:  ui.Rect{X: r.X + plotW, Y: r.Y, W: axisW, H: priceH},
		VolumeAxis: ui.Rect{X: r.X + plotW, Y: volY, W: axisW, H: volH},
		MACDAxis:   ui.Rect{X: r.X + plotW, Y: macdY, W: axisW, H: macdH},
	}
}
