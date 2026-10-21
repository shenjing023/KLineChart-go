package kline

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// MarkerSide is a trade marker's direction.
type MarkerSide int

const (
	// SideBuy is a buy. The marker points up from below the bar's low.
	SideBuy MarkerSide = iota + 1
	// SideSell is a sell. The marker points down from above the bar's high.
	SideSell
)

// Marker is one simulated trade on a symbol. Time selects the bar whose
// open interval contains it, so a period change and a forward prepend
// keep the same trade on the bar that covers that time. Several markers
// may share a time; they stack instead of replacing one another.
// Price 0 draws only the arrow at the bar. A non-zero Price also draws
// a tick at that price. An empty Label is B or S. Qty 0 is omitted.
type Marker struct {
	Time  time.Time
	Side  MarkerSide
	Price float64
	Label string
	Qty   float64
}

// ErrUnknownSymbol is returned when a marker call names a symbol the
// chart does not have.
var ErrUnknownSymbol = errors.New("kline: unknown symbol")

// ErrUnknownMarker is returned when Side is not SideBuy or SideSell.
// The markers already stored stay as they were.
var ErrUnknownMarker = errors.New("kline: unknown marker side")

// tradeMark is a marker plus the bar it currently lands on. bar -1 is
// outside the loaded candles and is not drawn. stack is how many earlier
// markers of the same side share that bar. text is the label painted
// beside the arrow, built when the marker is stored.
type tradeMark struct {
	m     Marker
	bar   int
	stack int
	text  string
}

// markerView is the period whose candles markers are mapped onto and,
// during replay, the exclusive end of the revealed base interval.
// bounded distinguishes an empty replay from a normal chart, where a
// zero until means there is no cutoff.
type markerView struct {
	period  Period
	from    time.Time
	until   time.Time
	bounded bool
}

const (
	markerH      float32 = 8
	markerW      float32 = 8
	markerGap    float32 = 3
	markerStride float32 = markerH + markerGap
)

// SetMarkers replaces symbol's markers. The slice is copied. An unknown
// symbol returns ErrUnknownSymbol. A side other than buy or sell returns
// ErrUnknownMarker and leaves the previous markers in place.
func (a *Chart) SetMarkers(symbol string, in []Marker) error {
	i := a.paneOf(symbol)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	for _, m := range in {
		if !knownSide(m.Side) {
			return fmt.Errorf("%w: %d", ErrUnknownMarker, m.Side)
		}
	}
	marks := make([]tradeMark, len(in))
	for j, m := range in {
		marks[j] = tradeMark{m: m, text: markerText(m)}
	}
	a.panes[i].marks = marks
	a.reindexMarks(&a.panes[i], a.markerView())
	return nil
}

// AppendMarker adds one marker on symbol. It does not replace the others.
func (a *Chart) AppendMarker(symbol string, m Marker) error {
	i := a.paneOf(symbol)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	if !knownSide(m.Side) {
		return fmt.Errorf("%w: %d", ErrUnknownMarker, m.Side)
	}
	a.panes[i].marks = append(a.panes[i].marks, tradeMark{m: m, text: markerText(m)})
	a.reindexMarks(&a.panes[i], a.markerView())
	return nil
}

// ClearMarkers removes every marker on symbol.
func (a *Chart) ClearMarkers(symbol string) error {
	i := a.paneOf(symbol)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	a.panes[i].marks = nil
	return nil
}

// Markers is a copy of the markers stored for symbol, in insertion order.
// An unknown symbol returns ErrUnknownSymbol.
func (a *Chart) Markers(symbol string) ([]Marker, error) {
	i := a.paneOf(symbol)
	if i < 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	out := make([]Marker, len(a.panes[i].marks))
	for j, m := range a.panes[i].marks {
		out[j] = m.m
	}
	return out, nil
}

func knownSide(side MarkerSide) bool {
	return side == SideBuy || side == SideSell
}

func markerText(m Marker) string {
	label := m.Label
	if label == "" {
		label = "B"
		if m.Side == SideSell {
			label = "S"
		}
	}
	if m.Qty != 0 && !math.IsNaN(m.Qty) {
		label += " " + strconv.FormatFloat(m.Qty, 'f', -1, 64)
	}
	return label
}

// markKey identifies one stack: a bar and a side. Markers with the same
// key step away from the candle in insertion order.
type markKey struct {
	bar  int
	side MarkerSide
}

// markerView is the mapping the chart currently exposes. A replay
// bounds it by the next hidden base open, so a partial aggregate bucket
// cannot reveal a marker whose base bar is still private.
func (a *Chart) markerView() markerView {
	view := markerView{period: a.period}
	if a.replay != nil {
		view = a.replay.markerView()
	}
	return view
}

// reindexMarks maps each visible marker onto the bar that contains its
// time and stacks markers that share a bar and a side. It writes the
// existing slice. The stack pass is one walk with a small map, so a bar
// carrying thousands of trades does not rescan the prefix for every
// marker.
func (a *Chart) reindexMarks(s *series, view markerView) {
	if s == nil {
		return
	}
	step, err := view.period.Duration()
	if err != nil {
		step = 0
	}
	seen := make(map[markKey]int, len(s.marks))
	for i := range s.marks {
		s.marks[i].bar = -1
		s.marks[i].stack = 0
		if view.bounded {
			if !view.from.IsZero() && s.marks[i].m.Time.Before(view.from) {
				continue
			}
			if !s.marks[i].m.Time.Before(view.until) {
				continue
			}
		}
		s.marks[i].bar = barContaining(s.candles, s.marks[i].m.Time, step)
		if s.marks[i].bar < 0 {
			continue
		}
		key := markKey{bar: s.marks[i].bar, side: s.marks[i].m.Side}
		s.marks[i].stack = seen[key]
		seen[key]++
	}
}

// barContaining is the bar whose half-open interval [open, next) holds t.
// The last bar ends one period later. t outside the series is -1.
func barContaining(cs []Candle, t time.Time, step time.Duration) int {
	if len(cs) == 0 {
		return -1
	}
	i := sort.Search(len(cs), func(i int) bool { return cs[i].Time.After(t) }) - 1
	if i < 0 {
		return -1
	}
	var end time.Time
	switch {
	case i+1 < len(cs):
		end = cs[i+1].Time
	case step > 0:
		end = cs[i].Time.Add(step)
	case i > 0:
		end = cs[i].Time.Add(cs[i].Time.Sub(cs[i-1].Time))
	case t.Equal(cs[0].Time):
		return 0
	default:
		return -1
	}
	if !t.Before(end) {
		return -1
	}
	return i
}

// markerTipY is the arrow tip. Buys sit below yLow and point up. Sells
// sit above yHigh and point down. stack steps away from the bar so
// markers on one bar do not overlap.
func markerTipY(side MarkerSide, yLow, yHigh float32, stack int) float32 {
	slot := float32(stack)
	if side == SideSell {
		return yHigh - markerGap - slot*markerStride
	}
	return yLow + markerGap + slot*markerStride
}

func markerTick(m tradeMark) bool {
	return m.m.Price != 0 && !math.IsNaN(m.m.Price)
}

func markerTickY(plot ui.Rect, scale chart.Linear, m tradeMark, factor float64) float32 {
	px := m.m.Price
	if factor != 0 && factor != 1 {
		px *= factor
	}
	return chart.Y(plot, scale.At(px))
}

// drawMarkers paints symbol pane's markers on the price plot. factor
// scales raw prices onto the scale argument; 1 is identity. Pan does not
// allocate for the markers: the triangle path is reused, and the label
// string was built when the marker was stored.
func (a *Chart) drawMarkers(p *ui.Painter, plot ui.Rect, pane int, cs []Candle, vp Viewport, scale chart.Linear, style Style, factor float64, anchorClose bool) {
	if a == nil || p == nil || pane < 0 || pane >= len(a.panes) || plot.H < 2 || len(cs) == 0 {
		return
	}
	marks := a.panes[pane].marks
	if len(marks) == 0 {
		return
	}
	if factor == 0 {
		factor = 1
	}
	first, last := vp.Visible(len(cs), float64(plot.W))
	if last < first {
		return
	}
	p.Clip(plot, 0, func() {
		for i := range marks {
			mk := &marks[i]
			if mk.bar < first || mk.bar > last || mk.bar >= len(cs) {
				continue
			}
			c := cs[mk.bar]
			low, high := c.Low*factor, c.High*factor
			if anchorClose {
				low, high = c.Close*factor, c.Close*factor
			}
			cx := plot.X + float32(vp.BarLeft(mk.bar)+vp.BarWidth/2)
			if cx < plot.X || cx > plot.X+plot.W {
				continue
			}
			yLow := chart.Y(plot, scale.At(low))
			yHigh := chart.Y(plot, scale.At(high))
			color := style.Buy
			if mk.m.Side == SideSell {
				color = style.Sell
			}
			tipY := markerTipY(mk.m.Side, yLow, yHigh, mk.stack)
			baseY := tipY + markerH
			if mk.m.Side == SideSell {
				baseY = tipY - markerH
			}
			path := a.markPath.Reset()
			path.MoveTo(cx, tipY)
			path.LineTo(cx-markerW/2, baseY)
			path.LineTo(cx+markerW/2, baseY)
			path.Close()
			p.FillPath(path, color)
			mid := (tipY + baseY) / 2
			p.Text(cx+markerW/2+2, mid-5, mk.text, 10, color)
			if markerTick(*mk) {
				y := markerTickY(plot, scale, *mk, factor)
				if y >= plot.Y && y <= plot.Y+plot.H {
					p.Line(cx-4, y, cx+4, y, 2, color)
				}
			}
		}
	})
}

// markerReadout is the header text for markers on bar idx. It is empty
// when that bar has none, and then it allocates nothing.
func markerReadout(marks []tradeMark, idx int, c Candle, labels labelFmt) string {
	var b strings.Builder
	n := 0
	for i := range marks {
		if marks[i].bar != idx {
			continue
		}
		if n > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(marks[i].text)
		px := marks[i].m.Price
		if px == 0 || math.IsNaN(px) {
			px = c.Low
			if marks[i].m.Side == SideSell {
				px = c.High
			}
		}
		b.WriteString(" @ ")
		b.WriteString(labels.price(px))
		n++
	}
	if n == 0 {
		return ""
	}
	return b.String()
}
