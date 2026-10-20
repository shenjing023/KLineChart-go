// K-line is a native candlestick demo. The window is ui.View, drawn by
// MyGo itself: no HTML and no WebView. Candles, volume, pan, zoom, and
// the crosshair are painted in this process; ui/chart only supplies
// the price and time scales.
package main

import (
	"log"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

const (
	sampleBars   = 600
	forwardBatch = 200
	// forwardEdge is the shared offset below which older bars are loaded.
	// Offset is the bar index at the left edge, the same value Viewport uses.
	forwardEdge = 5
)

const (
	toolPan = iota
	toolHLine
	toolSegment
)

const (
	// layoutStack is the default: one column of panes per symbol.
	layoutStack = iota
	// layoutOverlay draws both symbols on one price plot. The primary
	// series stays candles. The second series is a close line. Both are
	// indexed to 100 at the first visible common bar.
	layoutOverlay
)

// clickSlop is how far a pan-tool press can move, in DIPs, and still
// count as a click that selects an overlay. It matches MyGo's drag
// threshold.
const clickSlop = 4

// series is one symbol. Panes share one period, so a bar index is the
// same open time on each. inds is one instance per app slot, calculated
// for this symbol's bars.
type series struct {
	name    string
	candles []kline.Candle
	inds    []kline.Indicator
	more    bool
	// overlays are this pane's drawings. They are not shared with the
	// other symbol: a line placed here is not drawn on the other pane.
	overlays kline.Layer
}

// indicatorSlot is one entry in the app's indicator list. on is the
// toolbar toggle. The list is filled from kline.Registry at startup.
type indicatorSlot struct {
	spec kline.Spec
	on   bool
}

// app is the state the window shows. One viewport and one crossIndex
// are shared by every pane. Bars come from src.
type app struct {
	src    kline.DataSource
	period kline.Period
	panes  []series
	vp     kline.Viewport
	// fitted is set once a price plot has a width, so the first frame
	// opens on the latest bars instead of bar 0.
	fitted bool
	// plotW is the price-plot width from the last paint. Wheel and drag
	// on any pane use it; the panes are the same width.
	plotW float64
	// crossIndex is the bar under the pointer, or -1 when the pointer
	// is outside every pane. crossPane is the pane that has the pointer,
	// which is the only one that draws the horizontal line.
	crossIndex int
	crossPane  int
	hoverX     float32
	hoverY     float32
	// loading is true while a forward request is in flight. Synthetic
	// loads return immediately; the flag still drops a re-entrant call.
	loading bool
	// slots is the indicator list. Both symbols share the toggles and
	// keep their own calculated instances on series.inds.
	slots []indicatorSlot
	// painted is each symbol's layout from the latest frame.
	painted []kline.Panes
	// tool is Pan, H-Line, or Segment. Draw tools do not pan: the
	// pointer places and moves overlays instead.
	tool int
	// layout is Stack or Overlay. Stack is the zero value.
	layout int
	// draft is the first point of a segment that does not have its
	// second point yet. draftPane is the pane it belongs to.
	draftOn   bool
	draftPane int
	draft     kline.Point
	// drag moves an overlay the pointer went down on.
	dragOn   bool
	dragPane int
	dragID   string
	dragPart int
	// press is a pan-tool click, used to select on release when the
	// pointer barely moved. A drag keeps panning and does not select.
	pressOn   bool
	pressPane int
	pressX    float64
	pressY    float64
}

func newApp() *app {
	reg := kline.Registry()
	slots := make([]indicatorSlot, len(reg))
	for i, spec := range reg {
		// MA and MACD both start on. MACD's sub-pane is part of the
		// first frame; turning the toggle off drops that band.
		slots[i] = indicatorSlot{spec: spec, on: true}
	}
	a := &app{
		src:        &kline.SyntheticSource{},
		period:     kline.Period{Span: 1, Unit: kline.UnitHour},
		panes:      []series{{name: "ALPHA"}, {name: "BETA"}},
		slots:      slots,
		vp:         kline.Viewport{BarWidth: kline.DefaultBarWidth},
		crossIndex: -1,
		crossPane:  -1,
	}
	if err := a.setPeriod(a.period); err != nil {
		panic(err)
	}
	return a
}

// setPeriod reloads every pane with Kind=init and fits the latest bars
// when the plot width is already known. The first frame still fits from
// paint, because layout has not run yet.
func (a *app) setPeriod(p kline.Period) error {
	if a.src == nil {
		return kline.ErrInvalidPeriod
	}
	next := make([]series, len(a.panes))
	for i, s := range a.panes {
		bars, more, err := a.src.GetBars(kline.BarRequest{
			Symbol: s.name,
			Period: p,
			Kind:   kline.KindInit,
			Limit:  sampleBars,
		})
		if err != nil {
			return err
		}
		next[i] = a.mount(series{name: s.name}, bars, more)
	}
	a.period = p
	a.panes = next
	a.crossIndex = -1
	a.crossPane = -1
	a.fitted = false
	if a.plotW > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
	return nil
}

var periodChoices = []struct {
	label  string
	period kline.Period
}{
	{"1h", kline.Period{Span: 1, Unit: kline.UnitHour}},
	{"4h", kline.Period{Span: 4, Unit: kline.UnitHour}},
	{"1d", kline.Period{Span: 1, Unit: kline.UnitDay}},
}

// ensureForward prepends older bars when the shared offset is near the
// left edge. Offset increases by the prepended count, which is the same
// rule as Viewport: bar i moves to i+n, so (i+n) - (offset+n) is unchanged.
// A load already in flight does not start another.
func (a *app) ensureForward() {
	if a.loading || a.src == nil || !a.fitted || a.vp.Offset >= forwardEdge || !a.anyMore() {
		return
	}
	a.loading = true
	defer func() { a.loading = false }()

	type fetched struct {
		bars []kline.Candle
		more bool
		ok   bool
	}
	got := make([]fetched, len(a.panes))
	for i, s := range a.panes {
		if !s.more || len(s.candles) == 0 {
			continue
		}
		bars, more, err := a.src.GetBars(kline.BarRequest{
			Symbol: s.name,
			Period: a.period,
			Kind:   kline.KindForward,
			Anchor: s.candles[0].Time,
			Limit:  forwardBatch,
		})
		if err != nil {
			return
		}
		bars = strictlyBefore(bars, s.candles[0].Time)
		got[i] = fetched{bars: bars, more: more, ok: true}
	}
	n := 0
	have := false
	for _, g := range got {
		if !g.ok || len(g.bars) == 0 {
			continue
		}
		if !have || len(g.bars) < n {
			n = len(g.bars)
			have = true
		}
	}
	if !have || n == 0 {
		for i := range a.panes {
			if got[i].ok && len(got[i].bars) == 0 {
				a.panes[i].more = false
			}
		}
		return
	}
	for i := range a.panes {
		if !got[i].ok || len(got[i].bars) == 0 {
			if got[i].ok {
				a.panes[i].more = false
			}
			continue
		}
		older := got[i].bars
		if len(older) > n {
			older = older[len(older)-n:]
		}
		a.panes[i] = a.mount(a.panes[i], prependBars(older, a.panes[i].candles), got[i].more)
	}
	a.vp.Offset += float64(n)
	if a.crossIndex >= 0 {
		a.crossIndex += n
	}
}

func (a *app) anyMore() bool {
	for _, s := range a.panes {
		if s.more && len(s.candles) > 0 {
			return true
		}
	}
	return false
}

func strictlyBefore(bars []kline.Candle, anchor time.Time) []kline.Candle {
	end := len(bars)
	for end > 0 && !bars[end-1].Time.Before(anchor) {
		end--
	}
	return bars[:end]
}

func prependBars(older, newer []kline.Candle) []kline.Candle {
	out := make([]kline.Candle, 0, len(older)+len(newer))
	out = append(out, older...)
	out = append(out, newer...)
	return out
}

// mount stores bars on s and recalculates every indicator in the list.
// Existing instances are kept so a prepend does not allocate a new MA
// or MACD, only a new value slice inside Calc.
func (a *app) mount(s series, bars []kline.Candle, more bool) series {
	s.candles = bars
	s.more = more
	if len(s.inds) != len(a.slots) {
		s.inds = make([]kline.Indicator, len(a.slots))
		for i, slot := range a.slots {
			s.inds[i] = slot.spec.New()
		}
	}
	for _, ind := range s.inds {
		if ind != nil {
			ind.Calc(bars)
		}
	}
	return s
}

// active is the indicators whose toolbar toggle is on.
func (a *app) active(s series) []kline.Indicator {
	out := make([]kline.Indicator, 0, len(a.slots))
	for i, slot := range a.slots {
		if !slot.on || i >= len(s.inds) || s.inds[i] == nil {
			continue
		}
		out = append(out, s.inds[i])
	}
	return out
}

// subOpen reports whether a sub-pane indicator is toggled on. The layout
// keeps one sub-pane, which is MACD in this list.
func (a *app) subOpen() bool {
	for _, slot := range a.slots {
		if slot.on && slot.spec.Kind == kline.PaneSub {
			return true
		}
	}
	return false
}

func (a *app) bars() int {
	if len(a.panes) == 0 {
		return 0
	}
	n := len(a.panes[0].candles)
	for _, s := range a.panes[1:] {
		if len(s.candles) < n {
			n = len(s.candles)
		}
	}
	return n
}

func (a *app) view(c *ui.Context) {
	// Load from the offset the previous frame left behind, before headers
	// read the series, so a prepend and its offset shift land together.
	a.ensureForward()
	t := c.Theme()
	style := kline.StyleFrom(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(8, 14, 0, 14).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, "K-line").Bold().FontSize(16)
			if a.layout == layoutOverlay {
				ui.Textf(c, "%d bars · overlay", a.bars()).FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Textf(c, "%d bars · %d panes", a.bars(), len(a.panes)).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Textf(c, "Period %s", a.period).FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "MA5").FontSize(12).TextColor(style.MA5)
			ui.Text(c, "MA20").FontSize(12).TextColor(style.MA20)
			ui.Text(c, "DIF").FontSize(12).TextColor(style.MA5)
			ui.Text(c, "DEA").FontSize(12).TextColor(style.MA20)
			for i := range a.slots {
				ui.Toggle(c, &a.slots[i].on, a.slots[i].spec.Name)
			}
			for _, choice := range periodChoices {
				var b ui.Element
				if a.period == choice.period {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					_ = a.setPeriod(choice.period)
				}
			}
			for _, choice := range layoutChoices {
				var b ui.Element
				if a.layout == choice.layout {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					a.layout = choice.layout
					a.draftOn = false
					a.dragOn = false
				}
			}
			if a.layout == layoutOverlay {
				ui.Text(c, "idx 100").FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, "BETA line").FontSize(12).TextColor(style.Compare)
			}
			for _, choice := range toolChoices {
				var b ui.Element
				if a.tool == choice.tool {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					a.tool = choice.tool
					a.draftOn = false
					a.dragOn = false
				}
			}
			ui.Box(c).Grow(1)
			if ui.Button(c, "Reset view").Clicked() {
				a.vp.BarWidth = kline.DefaultBarWidth
				a.fitted = false
			}
		})
		hint := "Pan drags both panes. H-Line and Segment draw on the pane you click, and pan is off while a draw tool is selected. Delete removes the selected line. Right-click removes the line under the pointer."
		if a.layout == layoutOverlay {
			hint = "Overlay indexes both symbols to 100 at the first visible bar where both closes are positive. ALPHA stays candles. BETA is the close line. Volume and MACD are ALPHA only, and drawings stay on ALPHA. Pan and the wheel still move the shared viewport."
		}
		ui.Text(c, hint).Padding(4, 14, 6, 14).FontSize(12).TextColor(t.TextMuted)

		// Headers are built before the charts report the pointer, so they
		// show crossIndex from the previous frame. The charts record the
		// new index, and a changed index asks for one more frame so every
		// header catches up before the next input.
		nextIndex := -1
		nextPane := -1
		hoverX, hoverY := float32(0), float32(0)
		anyHover := false
		anyPress := false
		type chartSlot struct {
			key, label string
			pane       int
			showTime   bool
		}
		slots := make([]chartSlot, len(a.panes))
		for i, s := range a.panes {
			slots[i] = chartSlot{key: s.name, label: s.name + " chart", pane: i, showTime: i == len(a.panes)-1}
		}
		if a.layout == layoutOverlay {
			// One plot. Both headers stay, so the crosshair can show
			// each symbol's own values at the shared bar index.
			for i := range a.panes {
				a.symbolHeader(c, i)
			}
			slots = []chartSlot{{key: "compare", label: "Compare chart", pane: 0, showTime: true}}
		}
		for _, slot := range slots {
			if a.layout != layoutOverlay {
				a.symbolHeader(c, slot.pane)
			}
			chart := ui.Box(c.Key(slot.key)).Grow(1).WidthPercent(100).
				Label(slot.label).Cursor(ui.CursorCrosshair).Focusable()
			pane := slot.pane
			showTime := slot.showTime
			chart.HandleInput(func(ev ui.InputEvent) bool {
				return a.onChartInput(pane, ev)
			})
			// Draw tools take the pointer for placing and dragging lines.
			// Pan is the tool that moves the shared viewport.
			if dx, _, pressed := chart.Dragged(); pressed && a.tool == toolPan {
				anyPress = true
				if dx != 0 {
					a.vp.Pan(float64(dx), a.plotW, a.bars())
				}
			}
			if x, y, over := chart.PointerPosition(); over {
				anyHover = true
				nextPane = pane
				hoverX, hoverY = x, y
				if bar, ok := a.vp.IndexAt(float64(x), a.bars()); ok {
					nextIndex = bar
				}
			}
			chart.Draw(func(p *ui.Painter, r ui.Rect) {
				if a.layout == layoutOverlay {
					a.paintCompare(p, r, t)
					return
				}
				a.paintPane(p, r, t, pane, showTime)
			})
		}
		if !anyHover && !anyPress {
			nextIndex = -1
			nextPane = -1
		}
		if nextIndex != a.crossIndex || nextPane != a.crossPane || hoverX != a.hoverX || hoverY != a.hoverY {
			a.crossIndex = nextIndex
			a.crossPane = nextPane
			a.hoverX, a.hoverY = hoverX, hoverY
			c.AnimationFrame()
		}
	})
}

var toolChoices = []struct {
	label string
	tool  int
}{
	{"Pan", toolPan},
	{"H-Line", toolHLine},
	{"Segment", toolSegment},
}

var layoutChoices = []struct {
	label  string
	layout int
}{
	{"Stack", layoutStack},
	{"Overlay", layoutOverlay},
}

func (a *app) symbolHeader(c *ui.Context, i int) {
	if i < 0 || i >= len(a.panes) {
		return
	}
	s := &a.panes[i]
	t := c.Theme()
	ui.Row(c).Padding(2, 14, 2, 14).Gap(10).AlignItems(ui.Center).Children(func() {
		name := ui.Text(c, s.name).Bold().FontSize(13)
		if a.layout == layoutOverlay && i == 1 {
			name.TextColor(kline.StyleFrom(t).Compare)
		}
		ui.Text(c, a.readout(i)).Font("monospace").FontSize(12).NoWrap().Grow(1)
	})
}

// onChartInput takes the wheel, the draw tools, and Delete from one pane.
// MyGo delivers InputScroll in DIPs. Positive DY is the direction that
// scrolls content downward, and that zooms the bars out. Positive DX
// scrolls toward later bars. The viewport is shared, so the other pane
// moves with this one. Drawing does not take the wheel.
func (a *app) onChartInput(pane int, ev ui.InputEvent) bool {
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
func (a *app) pointerDown(pane int, ev ui.InputEvent) bool {
	x, y, in := a.plotLocal(pane, ev)
	if ev.Button == 1 {
		if !in {
			return false
		}
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
		if in {
			a.pressOn = true
			a.pressPane = pane
			a.pressX, a.pressY = x, y
		}
		return false
	}
	if !in {
		return false
	}
	hit, hitOK := a.hit(pane, x, y)
	switch a.tool {
	case toolHLine:
		if hitOK && a.overlayKind(pane, hit.ID) == kline.OverlayHLine {
			a.panes[pane].overlays.Select(hit.ID)
			a.startDrag(pane, hit.ID, kline.HitBody)
			return true
		}
		o := a.panes[pane].overlays.Add(kline.OverlayHLine, a.pointAt(pane, x, y))
		a.startDrag(pane, o.ID, kline.HitBody)
		return true
	case toolSegment:
		if a.draftOn && a.draftPane == pane {
			a.panes[pane].overlays.Add(kline.OverlaySegment, a.draft, a.pointAt(pane, x, y))
			a.draftOn = false
			a.dragOn = false
			return true
		}
		if hitOK && a.overlayKind(pane, hit.ID) == kline.OverlaySegment && hit.Part >= 0 {
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
	default:
		return false
	}
}

func (a *app) startDrag(pane int, id string, part int) {
	a.dragOn = true
	a.dragPane = pane
	a.dragID = id
	a.dragPart = part
}

func (a *app) pointerMove(pane int, ev ui.InputEvent) bool {
	if !a.dragOn || pane != a.dragPane || ev.Button != 0 {
		return false
	}
	x, y, _ := a.plotLocal(pane, ev)
	if a.dragPart < 0 {
		price := a.painted[pane].Price
		scale := a.priceScale(pane)
		return a.panes[pane].overlays.SetPrice(a.dragID, kline.LocalYToPrice(float64(price.H), y, scale))
	}
	return a.panes[pane].overlays.SetPoint(a.dragID, a.dragPart, a.pointAt(pane, x, y))
}

func (a *app) pointerUp(pane int, ev ui.InputEvent) bool {
	if a.dragOn && pane == a.dragPane && ev.Button == 0 {
		a.dragOn = false
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

func (a *app) overlayKind(pane int, id string) string {
	o := a.panes[pane].overlays.ByID(id)
	if o == nil {
		return ""
	}
	return o.Kind
}

func (a *app) hit(pane int, x, y float64) (kline.Hit, bool) {
	if pane < 0 || pane >= len(a.panes) || pane >= len(a.painted) {
		return kline.Hit{}, false
	}
	price := a.painted[pane].Price
	return a.panes[pane].overlays.Hit(x, y, float64(price.W), float64(price.H), a.vp, a.priceScale(pane), a.panes[pane].candles)
}

// plotLocal converts a pointer event to price-plot DIPs. The price plot
// starts at the chart box's top-left, so the event's local axes match it.
// in is false over the volume strip, the MACD pane, and the axes.
func (a *app) plotLocal(pane int, ev ui.InputEvent) (x, y float64, in bool) {
	if pane < 0 || pane >= len(a.painted) {
		return 0, 0, false
	}
	price := a.painted[pane].Price
	x, y = float64(ev.X), float64(ev.Y)
	in = price.H > 1 && x >= 0 && x <= float64(price.W) && y >= 0 && y <= float64(price.H)
	return x, y, in
}

func (a *app) priceScale(pane int) chart.Linear {
	if a.layout == layoutOverlay && len(a.panes) >= 1 {
		compare := []kline.Candle{}
		if len(a.panes) > 1 {
			compare = a.panes[1].candles
		}
		s := a.panes[0]
		_, raw, _ := kline.CompareScales(s.candles, compare, a.active(s), a.vp, a.plotW)
		return raw
	}
	s := a.panes[pane]
	return kline.MainScale(s.candles, a.active(s), a.vp, a.plotW)
}

// pointAt snaps the pointer to a bar index and a price. Time is that
// bar's open time, so a later prepend still finds the same bar.
func (a *app) pointAt(pane int, x, y float64) kline.Point {
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
	if idx >= 0 && idx < len(cs) {
		when = cs[idx].Time
	}
	return kline.Point{
		BarIndex: idx,
		Time:     when,
		Price:    kline.LocalYToPrice(float64(price.H), y, a.priceScale(pane)),
	}
}

func (a *app) paintPane(p *ui.Painter, r ui.Rect, t *ui.Theme, i int, showTime bool) {
	panes := kline.SplitStack(r, showTime, a.subOpen())
	if i >= len(a.painted) {
		next := make([]kline.Panes, i+1)
		copy(next, a.painted)
		a.painted = next
	}
	a.painted[i] = panes
	a.plotW = float64(panes.Price.W)
	// The plot width is only known once layout has run, which is inside
	// Draw. Fitting here puts the latest bars on screen on the first
	// frame instead of waiting for a second build.
	if !a.fitted && panes.Price.W > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
	a.vp.Clamp(a.bars(), a.plotW)
	ptr := kline.Pointer{}
	if a.crossPane == i {
		ptr = kline.Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	s := a.panes[i]
	style := kline.StyleFrom(t)
	active := a.active(s)
	kline.Paint(p, panes, s.candles, active, a.vp, ptr, a.crossIndex, style)
	var rubber *kline.Rubber
	if a.draftOn && a.draftPane == i && a.crossPane == i {
		rubber = &kline.Rubber{From: a.draft, X: float64(a.hoverX), Y: float64(a.hoverY)}
	}
	scale := kline.MainScale(s.candles, active, a.vp, a.plotW)
	kline.DrawOverlays(p, panes.Price, s.overlays.Items(), a.vp, scale, s.candles, style, rubber)
}

// paintCompare draws both symbols in one column. Volume, MACD, and
// drawings belong to the primary symbol. The second symbol is the
// close line painted by PaintCompare.
func (a *app) paintCompare(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	panes := kline.SplitStack(r, true, a.subOpen())
	a.painted = []kline.Panes{panes}
	a.plotW = float64(panes.Price.W)
	if !a.fitted && panes.Price.W > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
	a.vp.Clamp(a.bars(), a.plotW)
	ptr := kline.Pointer{}
	if a.crossPane == 0 {
		ptr = kline.Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	primary := a.panes[0]
	compare := []kline.Candle{}
	if len(a.panes) > 1 {
		compare = a.panes[1].candles
	}
	style := kline.StyleFrom(t)
	active := a.active(primary)
	indexed := kline.PaintCompare(p, panes, primary.candles, compare, active, a.vp, ptr, a.crossIndex, style)
	var rubber *kline.Rubber
	if a.draftOn && a.draftPane == 0 && a.crossPane == 0 {
		rubber = &kline.Rubber{From: a.draft, X: float64(a.hoverX), Y: float64(a.hoverY)}
	}
	scale := kline.MainScale(primary.candles, active, a.vp, a.plotW)
	if indexed {
		_, scale, _ = kline.CompareScales(primary.candles, compare, active, a.vp, a.plotW)
	}
	kline.DrawOverlays(p, panes.Price, primary.overlays.Items(), a.vp, scale, primary.candles, style, rubber)
}

func (a *app) readout(i int) string {
	if i < 0 || i >= len(a.panes) {
		return ""
	}
	s := a.panes[i]
	n := len(s.candles)
	if n == 0 {
		return "No bars"
	}
	idx := n - 1
	label := "Last"
	if a.crossIndex >= 0 && a.crossIndex < n {
		idx = a.crossIndex
		label = "Hover"
	}
	line := label + "  " + kline.FormatBar(s.candles[idx], math.NaN(), math.NaN())
	if a.layout == layoutOverlay {
		line += "  idx " + kline.FormatPrice(a.indexValue(i, idx))
		// The compare series is a close line. Its MA and MACD are not
		// drawn, so they stay off this header.
		if i != 0 {
			return line
		}
	}
	for _, ind := range a.active(s) {
		if v := ind.ValueAt(idx); v != "" {
			line += "  " + v
		}
	}
	return line
}

// indexValue is close[idx] / close[base] * 100 for one symbol. base is
// the first visible bar where every symbol has a positive close.
func (a *app) indexValue(pane, idx int) float64 {
	if pane < 0 || pane >= len(a.panes) {
		return math.NaN()
	}
	cs := a.panes[pane].candles
	if idx < 0 || idx >= len(cs) {
		return math.NaN()
	}
	base := a.compareBase()
	if base < 0 || base >= len(cs) || cs[base].Close <= 0 || math.IsNaN(cs[base].Close) {
		return math.NaN()
	}
	return cs[idx].Close / cs[base].Close * 100
}

func (a *app) compareBase() int {
	sets := make([][]kline.Candle, 0, len(a.panes))
	for _, s := range a.panes {
		sets = append(sets, s.candles)
	}
	first, last := a.vp.Visible(a.bars(), a.plotW)
	return kline.IndexBase(first, last, sets...)
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "K-line",
			Width:     1100,
			Height:    1000,
			MinWidth:  640,
			MinHeight: 720,
			StateKey:  "kline",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
