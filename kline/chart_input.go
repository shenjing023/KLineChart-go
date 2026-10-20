package kline

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// onChartInput takes the wheel, the draw tools, and Delete from one pane.
// MyGo delivers InputScroll in DIPs. Positive DY is the direction that
// scrolls content downward, and that zooms the bars out. Positive DX
// scrolls toward later bars. The viewport is shared, so the other pane
// moves with this one. Drawing does not take the wheel.
func (a *Chart) onChartInput(pane int, ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputScroll:
		if ev.DY != 0 {
			// ev.X is local to the pane. Each price plot starts at the
			// pane's left edge, so that x is the shared zoom anchor.
			a.vp.Zoom(float64(ev.DY), float64(ev.X), a.plotW, a.bars())
		}
		if ev.DX != 0 {
			a.vp.Pan(-float64(ev.DX), a.plotW, a.bars())
		}
		return ev.DX != 0 || ev.DY != 0
	case ui.InputKeyDown:
		if ev.Key == ui.KeyEscape {
			return a.cancelDraft()
		}
		if ev.Key != ui.KeyDelete {
			return false
		}
		return a.panes[pane].overlays.DeleteSelected()
	case ui.InputPointerDown:
		return a.pointerDown(pane, ev)
	case ui.InputPointerMove:
		return a.pointerMove(pane, ev)
	case ui.InputPointerUp:
		return a.pointerUp(pane, ev)
	default:
		return false
	}
}

// pointerDown places or selects an overlay. The secondary button deletes
// the overlay under it. In pan mode the press is left for Dragged, and a
// later release selects when the pointer did not move.
func (a *Chart) pointerDown(pane int, ev ui.InputEvent) bool {
	x, y, in := a.plotLocal(pane, ev)
	if ev.Button == 1 && a.cancelDraft() {
		return true
	}
	if !in {
		// The first press can arrive before Draw has a price rect.
		// Keep it and place it once that rect exists.
		if ev.Button <= 1 && a.tool != toolPan && !a.frameReady(pane) {
			a.pending = deferredPointer{on: true, pane: pane, button: ev.Button, x: x, y: y}
			return true
		}
		return false
	}
	if ev.Button == 1 {
		hit, ok := a.hit(pane, x, y)
		if !ok {
			return false
		}
		a.panes[pane].overlays.Delete(hit.ID)
		a.dragOn = false
		return true
	}
	if ev.Button != 0 {
		return false
	}
	if a.tool == toolPan {
		a.pressOn = true
		a.pressPane = pane
		a.pressX, a.pressY = x, y
		return false
	}
	hit, hitOK := a.hit(pane, x, y)
	switch a.tool {
	case toolHLine:
		return a.placeOne(pane, OverlayHLine, hit, hitOK, x, y)
	case toolSegment:
		return a.placeTwo(pane, OverlaySegment, hit, hitOK, x, y)
	case toolFib:
		return a.placeTwo(pane, OverlayFibonacci, hit, hitOK, x, y)
	case toolCustom:
		spec, ok := overlayByKind(a.customKind)
		if !ok {
			return false
		}
		if spec.Anchors <= 1 {
			return a.placeOne(pane, a.customKind, hit, hitOK, x, y)
		}
		return a.placeTwo(pane, a.customKind, hit, hitOK, x, y)
	default:
		return false
	}
}

func (a *Chart) placeOne(pane int, kind string, hit Hit, hitOK bool, x, y float64) bool {
	if hitOK && a.overlayKind(pane, hit.ID) == kind {
		a.panes[pane].overlays.Select(hit.ID)
		a.startDrag(pane, hit.ID, HitBody)
		return true
	}
	o := a.panes[pane].overlays.Add(kind, a.pointAt(pane, x, y))
	a.startDrag(pane, o.ID, HitBody)
	if kind == OverlayHLine {
		a.dragNewHLine = true
	}
	return true
}

func (a *Chart) placeTwo(pane int, kind string, hit Hit, hitOK bool, x, y float64) bool {
	if a.draftOn && a.draftPane == pane {
		a.panes[pane].overlays.Add(kind, a.draft, a.pointAt(pane, x, y))
		a.draftOn = false
		a.dragOn = false
		return true
	}
	if hitOK && a.overlayKind(pane, hit.ID) == kind && hit.Part >= 0 {
		a.panes[pane].overlays.Select(hit.ID)
		a.draftOn = false
		a.startDrag(pane, hit.ID, hit.Part)
		return true
	}
	if hitOK {
		a.panes[pane].overlays.Select(hit.ID)
		a.draftOn = false
		return true
	}
	a.draft = a.pointAt(pane, x, y)
	a.draftOn = true
	a.draftPane = pane
	a.dragOn = false
	return true
}

func (a *Chart) cancelDraft() bool {
	if !a.draftOn {
		return false
	}
	a.draftOn = false
	a.dragOn = false
	return true
}

func (a *Chart) startDrag(pane int, id string, part int) {
	a.dragOn = true
	a.dragPane = pane
	a.dragID = id
	a.dragPart = part
	a.dragNewHLine = false
}

func (a *Chart) pointerMove(pane int, ev ui.InputEvent) bool {
	if !a.dragOn || pane != a.dragPane || ev.Button != 0 {
		return false
	}
	x, y, _ := a.plotLocal(pane, ev)
	if a.dragPart < 0 {
		kind := a.overlayKind(pane, a.dragID)
		if kind != OverlayHLine {
			return a.panes[pane].overlays.SetPoint(a.dragID, 0, a.pointAt(pane, x, y))
		}
		if a.dragNewHLine {
			return a.panes[pane].overlays.SetPrice(a.dragID, a.pointAt(pane, x, y).Price)
		}
		price := a.painted[pane].Price
		scale := a.priceScale(pane)
		return a.panes[pane].overlays.SetPrice(a.dragID, LocalYToPrice(float64(price.H), y, scale))
	}
	return a.panes[pane].overlays.SetPoint(a.dragID, a.dragPart, a.pointAt(pane, x, y))
}

func (a *Chart) pointerUp(pane int, ev ui.InputEvent) bool {
	if a.dragOn && pane == a.dragPane && ev.Button == 0 {
		a.dragOn = false
		a.dragNewHLine = false
		return true
	}
	if a.tool == toolPan && a.pressOn && pane == a.pressPane && ev.Button == 0 {
		x, y, in := a.plotLocal(pane, ev)
		dx, dy := x-a.pressX, y-a.pressY
		a.pressOn = false
		if !in || dx*dx+dy*dy > clickSlop*clickSlop {
			return false
		}
		if hit, ok := a.hit(pane, x, y); ok {
			a.panes[pane].overlays.Select(hit.ID)
		} else {
			a.panes[pane].overlays.Select("")
		}
		return true
	}
	return false
}

func (a *Chart) overlayKind(pane int, id string) string {
	o := a.panes[pane].overlays.ByID(id)
	if o == nil {
		return ""
	}
	return o.Kind
}

func (a *Chart) draftRubberKind() string {
	if a.tool == toolFib {
		return OverlayFibonacci
	}
	if a.tool == toolCustom {
		return a.customKind
	}
	return ""
}

func (a *Chart) hit(pane int, x, y float64) (Hit, bool) {
	if pane < 0 || pane >= len(a.panes) || pane >= len(a.painted) {
		return Hit{}, false
	}
	price := a.painted[pane].Price
	return a.panes[pane].overlays.Hit(x, y, float64(price.W), float64(price.H), a.vp, a.priceScale(pane), a.panes[pane].candles)
}

func (a *Chart) rememberFrame(i int, box ui.Rect, showTime bool) {
	if i < 0 {
		return
	}
	if i >= len(a.frames) {
		next := make([]paneFrame, i+1)
		copy(next, a.frames)
		a.frames = next
	}
	a.frames[i] = paneFrame{box: box, showTime: showTime}
}

func (a *Chart) frameReady(i int) bool {
	return i >= 0 && i < len(a.frames) && a.frames[i].box.W > 1 && a.frames[i].box.H > 1
}

// syncPainted rebuilds pane i's rects from the last outer box and the
// indicators that are on now. A click in the same frame as SetIndicator
// then sees the price plot Draw is about to use.
func (a *Chart) syncPainted(i int) {
	if !a.frameReady(i) {
		return
	}
	panes := a.layoutPanes(a.frames[i].box, a.frames[i].showTime)
	if i >= len(a.painted) {
		next := make([]Panes, i+1)
		copy(next, a.painted)
		a.painted = next
	}
	a.painted[i] = panes
	if panes.Price.W > 1 {
		a.plotW = float64(panes.Price.W)
	}
}

// applyDeferred places a press that arrived before the first price rect.
// The drag that placement starts is cleared: the button is not held.
func (a *Chart) applyDeferred(pane int) {
	d := a.pending
	if !d.on || d.pane != pane {
		return
	}
	a.pending.on = false
	ev := ui.InputEvent{Kind: ui.InputPointerDown, Button: d.button, X: float32(d.x), Y: float32(d.y)}
	a.pointerDown(pane, ev)
	a.dragOn = false
	a.dragNewHLine = false
}

// plotLocal converts a pointer event to price-plot DIPs. The price plot
// starts at the chart box's top-left, so the event's local axes match it.
// in is false over the volume strip, the sub-panes, and the axes.
func (a *Chart) plotLocal(pane int, ev ui.InputEvent) (x, y float64, in bool) {
	a.syncPainted(pane)
	if pane < 0 || pane >= len(a.painted) {
		return 0, 0, false
	}
	price := a.painted[pane].Price
	x, y = float64(ev.X), float64(ev.Y)
	in = price.H > 1 && x >= 0 && x <= float64(price.W) && y >= 0 && y <= float64(price.H)
	return x, y, in
}

// priceScale maps the pane's price plot for hit tests and new points. In
// overlay it is CompareScales' raw scale, which is also what overlays are
// drawn with, including when the base cannot be indexed.
func (a *Chart) priceScale(pane int) chart.Linear {
	if a.layout == layoutOverlay && len(a.panes) >= 1 {
		primary, compare := a.overlayPair()
		_, raw, _ := CompareScales(primary, compare, a.active(&a.panes[0]), a.vp, a.plotW)
		return raw
	}
	s := &a.panes[pane]
	return MainScale(s.candles, a.active(s), a.vp, a.plotW)
}

// overlayPair is what LayoutOverlay draws: the first symbol as candles
// and the second as the indexed close line. Other symbols are not drawn
// there.
func (a *Chart) overlayPair() (primary, compare []Candle) {
	if len(a.panes) > 0 {
		primary = a.panes[0].candles
	}
	if len(a.panes) > 1 {
		compare = a.panes[1].candles
	}
	return primary, compare
}

// pointAt snaps the pointer to a bar index and a price. Time is that
// bar's open time, so a later prepend still finds the same bar.
func (a *Chart) pointAt(pane int, x, y float64) Point {
	cs := a.panes[pane].candles
	price := a.painted[pane].Price
	if price.W > 0 {
		if x < 0 {
			x = 0
		}
		if x > float64(price.W) {
			x = float64(price.W)
		}
	}
	idx := 0
	if n := len(cs); n > 0 {
		if i, ok := a.vp.IndexAt(x, n); ok {
			idx = i
		} else if a.vp.BarWidth > 0 {
			i := int(math.Floor(a.vp.Offset + x/a.vp.BarWidth))
			if i < 0 {
				i = 0
			}
			if i >= n {
				i = n - 1
			}
			idx = i
		}
	}
	var when time.Time
	scale := a.priceScale(pane)
	raw := LocalYToPrice(float64(price.H), y, scale)
	px := raw
	if idx >= 0 && idx < len(cs) {
		when = cs[idx].Time
		px = SnapPrice(a.magnet, cs[idx], raw, float64(price.H), scale)
	}
	return Point{
		BarIndex: idx,
		Time:     when,
		Price:    px,
	}
}
