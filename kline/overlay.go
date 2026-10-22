package kline

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// ErrDuplicateOverlay is returned when RegisterOverlay reuses a kind.
var ErrDuplicateOverlay = errors.New("kline: duplicate overlay")

// OverlayDrawFunc paints one overlay inside the price plot.
// It must not change o.Points: Chart passes the layer's own slice.
type OverlayDrawFunc func(p *ui.Painter, plot ui.Rect, o Overlay, vp Viewport, scale chart.Linear, cs []Candle, style Style)

// OverlayHitFunc reports the endpoint index under the pointer, or
// HitBody when the span is hit. Coordinates are price-plot DIPs.
type OverlayHitFunc func(o Overlay, x, y, plotW, plotH float64, vp Viewport, scale chart.Linear, cs []Candle) (part int, ok bool)

// OverlayRubberFunc paints the preview between the first click and the pointer.
type OverlayRubberFunc func(p *ui.Painter, plot ui.Rect, rubber Rubber, vp Viewport, scale chart.Linear, cs []Candle, style Style)

// OverlaySpec is a drawing tool a host can register. Kind is the name
// passed to SetTool. Anchors is 1 (place on click, drag to move) or 2
// (two clicks, drag an endpoint). Built-in kinds are reserved.
type OverlaySpec struct {
	Kind    string
	Anchors int
	Draw    OverlayDrawFunc
	Hit     OverlayHitFunc
	Rubber  OverlayRubberFunc
}

var (
	overlayMu  sync.RWMutex
	overlayReg []OverlaySpec
)

// RegisterOverlay adds a drawing tool. A repeated or reserved kind
// returns an error and does not replace the built-in tools.
// RegisterOverlay, UnregisterOverlay, and lookup from a draw can run
// on different goroutines.
func RegisterOverlay(spec OverlaySpec) error {
	if spec.Kind == "" || spec.Draw == nil || spec.Hit == nil {
		return fmt.Errorf("kline: overlay %q needs a kind, draw, and hit", spec.Kind)
	}
	if spec.Anchors != 1 && spec.Anchors != 2 {
		return fmt.Errorf("kline: overlay %q anchors must be 1 or 2", spec.Kind)
	}
	switch spec.Kind {
	case OverlayHLine, OverlaySegment, OverlayFibonacci, ToolPan, "fib":
		return fmt.Errorf("kline: overlay kind %q is reserved", spec.Kind)
	}
	overlayMu.Lock()
	defer overlayMu.Unlock()
	for _, have := range overlayReg {
		if have.Kind == spec.Kind {
			return fmt.Errorf("%w: %s", ErrDuplicateOverlay, spec.Kind)
		}
	}
	overlayReg = append(overlayReg, spec)
	return nil
}

// UnregisterOverlay removes a host-registered kind. A reserved or
// unknown kind is a no-op. A chart that already selected the tool
// keeps that selection until SetTool.
func UnregisterOverlay(kind string) {
	overlayMu.Lock()
	defer overlayMu.Unlock()
	next := make([]OverlaySpec, 0, len(overlayReg))
	for _, spec := range overlayReg {
		if spec.Kind != kind {
			next = append(next, spec)
		}
	}
	overlayReg = next
}

func overlayByKind(kind string) (OverlaySpec, bool) {
	overlayMu.RLock()
	defer overlayMu.RUnlock()
	for _, spec := range overlayReg {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return OverlaySpec{}, false
}

const (
	// OverlayHLine is a horizontal price line. It has one point; only
	// Price is used. The line spans the price plot of its pane.
	OverlayHLine = "hline"
	// OverlaySegment is a straight segment between two points.
	OverlaySegment = "segment"
	// OverlayFibonacci is a retracement between two anchors. Standard
	// ratios are drawn as horizontal lines across the anchor span.
	OverlayFibonacci = "fibonacci"
	// HitRadius is how close the pointer must be, in DIPs, to hit an
	// overlay. Endpoints use the same radius as the line.
	HitRadius = 6
	// HitBody marks a hit on the span of a line rather than an endpoint.
	HitBody = -1
)

// Point is one anchor. BarIndex and Price are the logical position.
// Time, when set, is that bar's open time and wins over BarIndex, so a
// prepend that shifts indexes does not move the point.
type Point struct {
	BarIndex int
	Time     time.Time
	Price    float64
}

// Index returns the bar this point sits on. A non-zero Time is looked up
// in cs. With no Time, BarIndex is used, including when cs is empty, so a
// hit test can run from the index alone.
func (pt Point) Index(cs []Candle) (int, bool) {
	if !pt.Time.IsZero() {
		if len(cs) == 0 {
			return 0, false
		}
		i := sort.Search(len(cs), func(i int) bool {
			return !cs[i].Time.Before(pt.Time)
		})
		if i < len(cs) && cs[i].Time.Equal(pt.Time) {
			return i, true
		}
		return 0, false
	}
	if pt.BarIndex < 0 {
		return 0, false
	}
	if len(cs) > 0 && pt.BarIndex >= len(cs) {
		return 0, false
	}
	return pt.BarIndex, true
}

// Overlay is one drawing. Points are logical, not pixels. Selected is
// the highlight. An overlay belongs to one pane's Layer and is not drawn
// on the others.
type Overlay struct {
	ID       string
	Kind     string
	Points   []Point
	Selected bool
	// Data is free-form state the host attached, passed back to a
	// registered tool's Draw and Hit. The built-in kinds ignore it. A
	// host that paints from something other than the points, such as a
	// flag on the bar, keeps it here.
	Data any
}

// Hit is an overlay under the pointer. Part is an endpoint index, or
// HitBody for the span of the line.
type Hit struct {
	ID   string
	Part int
}

// Layer is the overlay list of one pane. Panes do not share a layer:
// a line placed on one symbol stays on that symbol.
type Layer struct {
	items []Overlay
	seq   int
}

// Add appends an overlay and selects it. Earlier overlays on this layer
// lose the selection.
func (l *Layer) Add(kind string, points ...Point) Overlay {
	if l == nil {
		return Overlay{}
	}
	l.seq++
	for i := range l.items {
		l.items[i].Selected = false
	}
	o := Overlay{
		ID:       fmt.Sprintf("ov-%d", l.seq),
		Kind:     kind,
		Points:   append([]Point(nil), points...),
		Selected: true,
	}
	l.items = append(l.items, o)
	return o
}

// live is the overlays Chart paints. Callers must not change Points.
func (l *Layer) live() []Overlay {
	if l == nil {
		return nil
	}
	return l.items
}

// Items returns a copy of the overlays, including their points.
func (l *Layer) Items() []Overlay {
	if l == nil {
		return nil
	}
	out := make([]Overlay, len(l.items))
	for i, o := range l.items {
		out[i] = o
		out[i].Points = append([]Point(nil), o.Points...)
	}
	return out
}

// ByID returns the overlay with id, or nil. The pointer is into the
// layer and is invalid after an Add or Delete.
func (l *Layer) ByID(id string) *Overlay {
	if l == nil {
		return nil
	}
	for i := range l.items {
		if l.items[i].ID == id {
			return &l.items[i]
		}
	}
	return nil
}

// Selected returns the selected overlay, or nil.
func (l *Layer) Selected() *Overlay {
	if l == nil {
		return nil
	}
	for i := range l.items {
		if l.items[i].Selected {
			return &l.items[i]
		}
	}
	return nil
}

// Select marks id and clears every other selection on this layer.
// An unknown id clears the selection.
func (l *Layer) Select(id string) {
	if l == nil {
		return
	}
	for i := range l.items {
		l.items[i].Selected = l.items[i].ID == id
	}
}

// Delete removes id. It reports whether an overlay was removed.
func (l *Layer) Delete(id string) bool {
	if l == nil {
		return false
	}
	for i := range l.items {
		if l.items[i].ID != id {
			continue
		}
		l.items = append(l.items[:i], l.items[i+1:]...)
		return true
	}
	return false
}

// DeleteSelected removes the selected overlay.
func (l *Layer) DeleteSelected() bool {
	sel := l.Selected()
	if sel == nil {
		return false
	}
	return l.Delete(sel.ID)
}

// SetPoint replaces one anchor. part is the endpoint index.
func (l *Layer) SetPoint(id string, part int, pt Point) bool {
	o := l.ByID(id)
	if o == nil || part < 0 || part >= len(o.Points) {
		return false
	}
	o.Points[part] = pt
	return true
}

// SetPrice sets the price of a horizontal line. A segment's ends move
// through SetPoint, one endpoint at a time.
func (l *Layer) SetPrice(id string, price float64) bool {
	o := l.ByID(id)
	if o == nil || o.Kind != OverlayHLine || len(o.Points) == 0 {
		return false
	}
	o.Points[0].Price = price
	return true
}

// Clear drops every overlay on this pane.
func (l *Layer) Clear() {
	if l == nil {
		return
	}
	l.items = nil
}

// PriceToLocalY maps price to a Y measured from the top of the price plot.
// chart.Linear grows upward; local Y grows downward.
func PriceToLocalY(plotH, price float64, scale chart.Linear) float64 {
	if plotH <= 0 {
		return 0
	}
	return float64(1-scale.At(price)) * plotH
}

// LocalYToPrice maps a Y measured from the top of the price plot to a price.
func LocalYToPrice(plotH, localY float64, scale chart.Linear) float64 {
	if plotH <= 0 {
		return scale.Lo
	}
	return scale.Invert(float32(1 - localY/plotH))
}

// AnchorX is the horizontal center of the point's bar, in DIPs from the
// left of the price plot.
func AnchorX(pt Point, vp Viewport, cs []Candle) float64 {
	i, ok := pt.Index(cs)
	if !ok {
		i = pt.BarIndex
	}
	if vp.BarWidth <= 0 {
		vp.BarWidth = DefaultBarWidth
	}
	return vp.BarLeft(i) + vp.BarWidth/2
}

// HitOverlay reports whether (x, y), in price-plot DIPs, lands on o.
// x and y are measured from the top-left of the price plot.
func HitOverlay(o Overlay, x, y, plotW, plotH float64, vp Viewport, scale chart.Linear, cs []Candle) (part int, ok bool) {
	switch o.Kind {
	case OverlayHLine:
		return hitHLine(o, x, y, plotW, plotH, scale)
	case OverlaySegment:
		return hitSegment(o, x, y, plotW, plotH, vp, scale, cs)
	case OverlayFibonacci:
		return hitFibonacci(o, x, y, plotW, plotH, vp, scale, cs)
	default:
		if spec, ok := overlayByKind(o.Kind); ok && spec.Hit != nil {
			return spec.Hit(o, x, y, plotW, plotH, vp, scale, cs)
		}
		return 0, false
	}
}

// Rubber is the unfinished end of a drawing: From is the first click,
// and X, Y are the pointer in price-plot DIPs. Kind is the tool being
// placed; the zero value keeps the segment rubber band.
type Rubber struct {
	From Point
	X, Y float64
	Kind string
}

// DrawOverlays paints items on the price plot. rubber, when set, is the
// segment that does not have its second point yet. Coordinates in rubber
// are local to plot, whose origin is the top-left. Draw funcs must not
// change item points; Chart passes the layer's own slice.
func DrawOverlays(p *ui.Painter, plot ui.Rect, items []Overlay, vp Viewport, scale chart.Linear, cs []Candle, style Style, rubber *Rubber) {
	if p == nil || plot.W < 2 || plot.H < 2 {
		return
	}
	p.Clip(plot, 0, func() {
		for _, o := range items {
			switch o.Kind {
			case OverlayHLine:
				drawHLine(p, plot, o, scale, style)
			case OverlaySegment:
				drawSegment(p, plot, o, vp, scale, cs, style)
			case OverlayFibonacci:
				drawFibonacci(p, plot, o, vp, scale, cs, style)
			default:
				if spec, ok := overlayByKind(o.Kind); ok && spec.Draw != nil {
					spec.Draw(p, plot, o, vp, scale, cs, style)
				}
			}
		}
		if rubber != nil {
			drawRubber(p, plot, *rubber, vp, scale, cs, style)
		}
	})
}

// Hit returns the topmost overlay under (x, y). Later overlays win when
// several contain the point.
func (l *Layer) Hit(x, y, plotW, plotH float64, vp Viewport, scale chart.Linear, cs []Candle) (Hit, bool) {
	if l == nil {
		return Hit{}, false
	}
	for i := len(l.items) - 1; i >= 0; i-- {
		part, ok := HitOverlay(l.items[i], x, y, plotW, plotH, vp, scale, cs)
		if ok {
			return Hit{ID: l.items[i].ID, Part: part}, true
		}
	}
	return Hit{}, false
}

func distToSegment(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / len2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}
