package kline

import (
	"math"
	"time"
)

const (
	// DefaultBarWidth is the DIP width of one bar, including the gap
	// before the next bar, when the chart first opens.
	DefaultBarWidth = 8
	// MinBarWidth and MaxBarWidth bound the wheel zoom.
	MinBarWidth = 3
	MaxBarWidth = 28
	// OverscrollBars is how many bar-widths of empty plot are allowed
	// before the first bar and after the last.
	OverscrollBars = 2
)

// Viewport is the horizontal window onto a series.
//
// Offset is the bar index that sits on the left edge of the price plot.
// It is fractional so a drag can move the series by less than one bar,
// and it may be negative when empty plot shows to the left of bar 0.
//
// Bar i's left edge, in DIPs from the price plot's left edge, is
//
//	(i - Offset) * BarWidth
//
// and the bar occupies the next BarWidth DIPs. BarWidth includes the gap
// between bodies; the body itself is a fraction of that (see Paint).
type Viewport struct {
	Offset   float64
	BarWidth float64
}

// FitRight places the last bar OverscrollBars in from the right edge.
// plotW is the price plot width in DIPs. A non-positive width is ignored.
func (v *Viewport) FitRight(n int, plotW float64) {
	if n <= 0 || plotW <= 0 {
		return
	}
	if v.BarWidth <= 0 {
		v.BarWidth = DefaultBarWidth
	}
	margin := float64(OverscrollBars) * v.BarWidth
	// Right edge of bar n-1 is at index n. Put that edge margin DIPs
	// left of the plot's right edge.
	v.Offset = float64(n) - (plotW-margin)/v.BarWidth
	v.Clamp(n, plotW)
}

// Pan slides the series by dx DIPs. Positive dx follows a pointer moving
// right: bars move right, so Offset decreases and earlier bars enter
// from the left.
func (v *Viewport) Pan(dx, plotW float64, n int) {
	if v.BarWidth <= 0 || dx == 0 {
		return
	}
	v.Offset -= dx / v.BarWidth
	v.Clamp(n, plotW)
}

// Zoom changes BarWidth, keeping the bar under anchorX fixed.
// anchorX is DIPs from the price plot's left edge. dy is a wheel delta
// in DIPs: positive (wheel down, the direction that scrolls content
// downward) zooms out, negative zooms in. A notch is on the order of
// 50 DIPs on Linux and about 100 DIPs on Windows.
func (v *Viewport) Zoom(dy, anchorX, plotW float64, n int) {
	if v.BarWidth <= 0 || plotW <= 0 || dy == 0 {
		return
	}
	next := v.BarWidth * math.Exp(-dy/260)
	if next < MinBarWidth {
		next = MinBarWidth
	}
	if next > MaxBarWidth {
		next = MaxBarWidth
	}
	if anchorX < 0 {
		anchorX = 0
	}
	if anchorX > plotW {
		anchorX = plotW
	}
	// Index under the pointer before the width changes.
	idx := v.Offset + anchorX/v.BarWidth
	v.BarWidth = next
	v.Offset = idx - anchorX/v.BarWidth
	v.Clamp(n, plotW)
}

// Clamp keeps BarWidth in range and Offset where at least part of the
// series can still meet the plot. A series shorter than the plot,
// including an empty one, is pinned to the left margin.
func (v *Viewport) Clamp(n int, plotW float64) {
	if v.BarWidth < MinBarWidth {
		v.BarWidth = MinBarWidth
	}
	if v.BarWidth > MaxBarWidth {
		v.BarWidth = MaxBarWidth
	}
	if plotW <= 0 {
		return
	}
	minOff := -float64(OverscrollBars)
	maxOff := float64(n) - plotW/v.BarWidth + float64(OverscrollBars)
	if n <= 0 || maxOff < minOff {
		v.Offset = minOff
		return
	}
	if v.Offset < minOff {
		v.Offset = minOff
	}
	if v.Offset > maxOff {
		v.Offset = maxOff
	}
}

// Visible returns the inclusive bar indexes that intersect the plot.
// last < first means nothing is visible.
func (v Viewport) Visible(n int, plotW float64) (first, last int) {
	if n <= 0 || plotW <= 0 || v.BarWidth <= 0 {
		return 0, -1
	}
	first = int(math.Floor(v.Offset))
	last = int(math.Ceil(v.Offset+plotW/v.BarWidth)) - 1
	if first < 0 {
		first = 0
	}
	if last >= n {
		last = n - 1
	}
	if last < first {
		return 0, -1
	}
	return first, last
}

// IndexAt returns the bar under localX, DIPs from the price plot's left
// edge. ok is false when that index is outside the series.
func (v Viewport) IndexAt(localX float64, n int) (int, bool) {
	if n <= 0 || v.BarWidth <= 0 {
		return 0, false
	}
	i := int(math.Floor(v.Offset + localX/v.BarWidth))
	if i < 0 || i >= n {
		return 0, false
	}
	return i, true
}

// BarLeft is the left edge of bar i, in DIPs from the price plot's left.
func (v Viewport) BarLeft(i int) float64 {
	return (float64(i) - v.Offset) * v.BarWidth
}

// TimeAt maps a fractional bar index onto the series clock. Indexes
// outside the series extrapolate with the first bar's spacing, so axis
// labels still move while the plot shows empty space past either end.
func TimeAt(cs []Candle, index float64) (t time.Time) {
	if len(cs) == 0 {
		return time.Time{}
	}
	if len(cs) == 1 {
		return cs[0].Time
	}
	step := cs[1].Time.Sub(cs[0].Time)
	if index <= 0 {
		return cs[0].Time.Add(time.Duration(index * float64(step)))
	}
	last := float64(len(cs) - 1)
	if index >= last {
		return cs[len(cs)-1].Time.Add(time.Duration((index - last) * float64(step)))
	}
	i := int(index)
	frac := index - float64(i)
	return cs[i].Time.Add(time.Duration(frac * float64(cs[i+1].Time.Sub(cs[i].Time))))
}

// PriceDomain is the padded high/low of bars first..last, widened to
// include any finite extra series (moving averages) in that window.
// Each bar contributes its finite Open, High, Low, and Close, so one NaN
// or infinite price drops out instead of turning the domain into NaN; a
// bar whose Low is missing still keeps its body on the scale. An empty
// window, or one with no finite price, returns 0, 1.
func PriceDomain(cs []Candle, first, last int, extra ...[]float64) (lo, hi float64) {
	if first < 0 {
		first = 0
	}
	if last >= len(cs) {
		last = len(cs) - 1
	}
	if first > last || len(cs) == 0 {
		return 0, 1
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for i := first; i <= last; i++ {
		c := cs[i]
		for _, v := range [4]float64{c.Open, c.High, c.Low, c.Close} {
			if finite(v) {
				lo = math.Min(lo, v)
				hi = math.Max(hi, v)
			}
		}
		for _, series := range extra {
			if i >= len(series) {
				continue
			}
			v := series[i]
			if !finite(v) {
				continue
			}
			lo = math.Min(lo, v)
			hi = math.Max(hi, v)
		}
	}
	if lo > hi || math.IsInf(lo, 0) {
		return 0, 1
	}
	span := hi - lo
	if span < 1e-6 {
		span = math.Max(math.Abs(hi)*0.02, 1)
		lo -= span / 2
		hi += span / 2
		span = hi - lo
	}
	pad := span * 0.08
	return lo - pad, hi + pad
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// VolumeMax is a little above the largest volume in first..last, or 1
// when the window is empty.
func VolumeMax(cs []Candle, first, last int) float64 {
	if first < 0 {
		first = 0
	}
	if last >= len(cs) {
		last = len(cs) - 1
	}
	if first > last || len(cs) == 0 {
		return 1
	}
	maxV := 0.0
	for i := first; i <= last; i++ {
		if cs[i].Volume > maxV {
			maxV = cs[i].Volume
		}
	}
	if maxV <= 0 {
		return 1
	}
	return maxV * 1.08
}
