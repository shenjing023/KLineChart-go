package kline

import (
	"math"
	"sort"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	sampleBars   = 600
	forwardBatch = 200
	// forwardEdge is the shared offset below which older bars are loaded.
	// Offset is the bar index at the left edge, the same value Viewport uses.
	forwardEdge = 5
	// forwardBackoff is how long ensureForward waits after a GetBars error,
	// or after a forward batch that cannot be shared, before calling the
	// source again. View calls ensureForward on every frame.
	forwardBackoff = 500 * time.Millisecond
)

const (
	toolPan = iota
	toolHLine
	toolSegment
	toolFib
	toolCustom
)

const (
	// layoutStack is the default: one column of panes per symbol.
	layoutStack = iota
	// layoutOverlay draws the first two symbols on one price plot. The
	// primary series stays candles. The second series is a close line.
	// Both are indexed to 100 at the first visible common bar.
	layoutOverlay
)

// clickSlop is how far a pan-tool press can move, in DIPs, and still
// count as a click that selects an overlay. It matches MyGo's drag
// threshold.
const clickSlop = 4

// series is one symbol. Every pane holds the same number of candles, and
// candle i is the same open time on each, so one viewport index addresses
// all of them. held are live bars newer than candles that wait until every
// symbol has a bar at the same open time. inds is one instance per app
// slot, calculated for this symbol's candles.
type series struct {
	name    string
	candles []Candle
	held    []Candle
	inds    []Indicator
	// fresh[i] is set while inds[i] holds values for the current candles.
	// An indicator that is off is not calculated, so it goes stale.
	fresh []bool
	more  bool
	// overlays are this pane's drawings. They are not shared with the
	// other symbol: a line placed here is not drawn on the other pane.
	overlays Layer
	// marks are this pane's trade markers, keyed by bar time. They stay
	// with the symbol across a period change and a forward prepend.
	marks []tradeMark
}

// indicatorSlot is one entry in the app's indicator list. on is the
// toolbar toggle. IndicatorEnabled hands out its address, so a slot is
// shared by pointer and never copied. The list is filled from Registry
// at startup.
type indicatorSlot struct {
	spec Spec
	on   bool
}

// Chart is an embeddable candlestick view. One viewport and one
// crossIndex are shared by every pane. Bars come from src. The host
// draws window chrome and the toolbar; View paints the plots.
//
// A nil *Chart is not usable: the getters and setters dereference the
// receiver. Only Close accepts nil, so a host may defer it before it has
// built a chart.
type Chart struct {
	src    DataSource
	period Period
	panes  []series
	vp     Viewport
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
	slots []*indicatorSlot
	// painted is each symbol's layout from the latest frame.
	painted []Panes
	// tool is Pan, H-Line, Segment, Fib, or a host-registered kind.
	// Draw tools do not pan: the pointer places and moves overlays instead.
	tool int
	// customKind is the registered overlay kind when tool is toolCustom.
	customKind string
	// hideHeader skips the symbol readouts View normally paints. The
	// host reads them from Readout.
	hideHeader bool
	// magnet snaps new points, including the press that places a new
	// H-Line, and endpoint drags on Segment and Fib. Dragging an
	// existing H-Line body uses the raw pointer price.
	magnet MagnetMode
	// layout is Stack or Overlay. Stack is the zero value.
	layout int
	// hollowUp draws rising candle bodies as outlines. Falling bodies
	// stay filled. Off, both are filled.
	hollowUp bool
	// style, when styleSet, replaces theme colors. hollowUp is the live
	// hollow switch. SetHollow writes it through to style.HollowUp
	// when a style is stored, matching SetLimitPct. limitPct is the
	// live limit threshold Paint reads, including when the colors
	// still come from the theme.
	style    Style
	styleSet bool
	limitPct float64
	// draft is the first point of a segment that does not have its
	// second point yet. draftPane is the pane it belongs to.
	draftOn   bool
	draftPane int
	draft     Point
	// drag moves an overlay the pointer went down on.
	dragOn   bool
	dragPane int
	dragID   string
	dragPart int
	// dragNewHLine is set only when this press places a new H-Line and
	// starts its drag. pointerMove then keeps the magnet price. Dragging
	// an existing H-Line body leaves it false.
	dragNewHLine bool
	// press is a pan-tool click, used to select on release when the
	// pointer barely moved. A drag keeps panning and does not select.
	pressOn   bool
	pressPane int
	pressX    float64
	pressY    float64
	// live is the LiveSource followLive subscribed, kept so a later
	// source swap can unsubscribe that source. Nil when src is only
	// a DataSource, which is the synthetic demo.
	live     LiveSource
	liveSubs []liveSub
	// liveGen fences handlers. dropLive bumps it so a callback that
	// outlives UnsubscribeBar does not apply. Push may run off the UI
	// goroutine, so this is atomic.
	liveGen atomic.Uint64
	// replay, when set, owns the full series. The panes hold only the
	// revealed prefix. View ticks it. SetPeriod aggregates that prefix
	// instead of asking the source, which would load bars past the cursor.
	replay *Replay
	// marshal is ChartOptions.Marshal. Nil applies a live bar on the caller.
	marshal func(func())
	// forwardHold is the earliest time a failed forward load may try again.
	// The zero time means there is no hold.
	forwardHold time.Time
	// loadErr is LoadError: why the bars of the current period and source
	// are missing or not live. Nil once they loaded and subscribed.
	loadErr error
	// alignErr is AlignError: forward or live bars left off the shared
	// index since the last load.
	alignErr error
	// closed records that Close dropped the live subscription. A
	// same-period SetPeriod must load again while this is set; a
	// successful load and subscription clears it.
	closed bool
	// symbolErr is set when NewChart refused the symbol list. The chart
	// then has no panes, and every load returns it.
	symbolErr error
	// priceDigits is the Readout and crosshair precision. digitsSet
	// distinguishes a whole-number precision of 0 from a zero Chart,
	// which still prints two decimals.
	priceDigits int
	digitsSet   bool
	// zone formats bar times in the header and the crosshair tag. It is
	// also the calendar a daily aggregate bucket starts on. Nil is UTC.
	zone *time.Location
	// timeFormat is the host's formatter for every printed time. Nil
	// keeps the built-in layouts. It is called with the instant already
	// in zone, so it never converts a time itself.
	timeFormat func(time.Time) string
	// subDraw is the host's sub-pane painter. Nil draws nothing.
	subDraw func(p *ui.Painter, rect ui.Rect, ind Indicator, index int)
	// tickFormat is the host's per-role time formatter. It wins over
	// timeFormat when both are set.
	tickFormat func(time.Time, TickKind) string
	// volFormat is the host's formatter for a volume-axis tick. Nil
	// prints the plain number.
	volFormat func(float64) string
	// barColor is the host's per-bar color rule. Nil, or a zero color for
	// a bar, falls back to Style.CandleBodyColor.
	barColor func(i int, c, prev Candle) ui.Color
	// crossHook is called when the hovered bar changes. It runs inside
	// View, so a host may move a second chart to the same time there.
	crossHook func(symbol string, index int, ok bool)
	// maPeriods and maColors override the built-in MA lines. Empty means
	// the registered defaults.
	maPeriods []int
	maColors  []ui.Color
	// displayNames are the accessible names, parallel to panes. An
	// empty entry falls back to the symbol.
	displayNames []string
	// paneHeights is the host's DIP override of the sub-pane bands. A
	// zero field keeps the built-in share of the plot.
	paneHeights PaneHeights
	// crossAt is a crosshair a host placed by time. It wins over the
	// pointer until the pointer moves or the host clears it, which is how
	// two charts share one cursor when each has its own pointer.
	crossAt    time.Time
	crossAtSet bool
	// crossFixed is true while crossAt is a host-placed crosshair, so a
	// frame with no pointer does not clear it.
	crossFixed bool
	// frames is the outer box of each pane from the last paint. Hit
	// testing relayouts from it so a click after an indicator toggle
	// uses the new price rect before the next Draw.
	frames []paneFrame
	// pending is a press that arrived before any price rect existed.
	// The next paint applies it once the plot size is known.
	pending deferredPointer
	// markPath is reused for every trade-marker triangle so a pan
	// frame does not allocate a path per marker.
	markPath ui.Path
}

// axisWidth is the axis column width. It is the built-in constant, not a
// per-frame measurement: a width that changed after the first paint would
// move the plot under a pointer already mapped to it, so a click would
// land on the wrong bar.
func (a *Chart) axisWidth() float32 { return axisWidth }

// paneFrame is one chart box from the last paint.
type paneFrame struct {
	box      ui.Rect
	showTime bool
}

// deferredPointer is a press held until paint knows the price rect.
type deferredPointer struct {
	on     bool
	pane   int
	button int
	x, y   float64
}

func priceDigitsOf(digits *int) int {
	if digits == nil || *digits < 0 {
		return 2
	}
	return *digits
}

func (a *Chart) labels() labelFmt {
	if a == nil || !a.digitsSet {
		return defaultLabels()
	}
	return labelFmt{digits: a.priceDigits, loc: a.zone, format: a.timeFormat, tick: a.tickFormat, volume: a.volFormat}
}

// bucketZone is the calendar a daily replay aggregate starts on. It is
// the zone the labels use, so the aggregate's open time is the day the
// header shows.
func (a *Chart) bucketZone() *time.Location {
	if a == nil {
		return nil
	}
	return a.zone
}

// active is s's indicators whose toolbar toggle is on, calculated for
// its candles. One that was off when the candles last changed is
// calculated here, once.
func (a *Chart) active(s *series) []Indicator {
	a.ensureInds(s)
	out := make([]Indicator, 0, len(a.slots))
	for i, slot := range a.slots {
		ind := s.inds[i]
		if !slot.on || ind == nil {
			continue
		}
		if !s.fresh[i] {
			ind.Calc(s.candles)
			s.fresh[i] = true
		}
		out = append(out, ind)
	}
	return out
}

// enabledSubs is the sub-pane indicators that are on, in toolbar order.
// The instances come from the first symbol. Every symbol mounts the
// same list, so the kinds match.
func (a *Chart) enabledSubs() []Indicator {
	if len(a.panes) == 0 {
		return nil
	}
	var out []Indicator
	for _, ind := range a.active(&a.panes[0]) {
		if ind != nil && ind.PaneKind() == PaneSub {
			out = append(out, ind)
		}
	}
	return out
}

// layoutPanes keeps the built-in MACD and KDJ geometry when those are
// the only sub-panes. A host sub-pane is an extra band.
func (a *Chart) layoutPanes(r ui.Rect, showTime bool) Panes {
	subs := a.enabledSubs()
	n := len(subs)
	var panes Panes
	switch n {
	case 0:
		// SplitStack leaves a zero-height MACD rect. Keep it so this
		// layout stays identical to the built-in no-sub-pane split.
		panes = splitAxis(r, showTime, false, false, a.axisWidth())
	case 1:
		panes = assignSubRects(splitAxis(r, showTime, true, false, a.axisWidth()), subs, n)
	case 2:
		panes = assignSubRects(splitAxis(r, showTime, true, true, a.axisWidth()), subs, n)
	default:
		panes = assignSubRects(splitManyAxis(r, showTime, n, a.axisWidth()), subs, n)
	}
	if a.paneHeights.Sub > 0 || a.paneHeights.Volume > 0 {
		panes = applyPaneHeights(r, panes, showTime, a.paneHeights, subs)
	}
	return panes
}

// applyPaneHeights re-splits a box whose bands the host set in DIPs.
// The price plot takes what is left. If the requested bands do not fit,
// they shrink in proportion and the price plot keeps a quarter of the
// body, so a host that asks for too much still gets a usable chart
// rather than an error it cannot act on.
//
// Band placement follows subBands, the same order layoutPanes used, so
// the host only changes heights and never which indicator paints where.
// The fallback's rects are used only for their built-in heights.
func applyPaneHeights(r ui.Rect, fallback Panes, showTime bool, h PaneHeights, inds []Indicator) Panes {
	order := subBands(inds)
	subs := len(inds)
	// One gap between price and volume, then one per sub-pane, then one
	// before the time strip. The strip and that last gap both come out
	// of the box before the bands are sized, so the bands fill exactly
	// what is left and nothing overruns into fitPanes.
	bands := 1 + subs
	bodyH := r.H
	if showTime {
		bodyH -= timeHeight
		if fallback.Time.H > 0 {
			bodyH -= paneGap
			bands++
		}
	}
	gaps := float32(paneGap) * float32(bands-1)
	inner := bodyH - gaps
	if inner <= 0 {
		return fallback
	}
	volH := fallback.Volume.H
	if h.Volume > 0 {
		volH = float32(h.Volume)
	}
	// Heights follow subBands, the same order layoutPanes used, not the
	// fallback's MACD/KDJ fields. A single open sub-pane borrows the
	// MACD geometry before assignSubRects moves it onto KDJ, so the
	// fallback's KDJ rect is empty even though the KDJ band is the one
	// being drawn. Reading the fallback fields directly dropped that
	// band into Extra and left the sub-pane blank.
	var heights []float32
	for i := range order {
		switch order[i] {
		case bandMACD:
			heights = append(heights, fallback.MACD.H)
		case bandKDJ:
			// A single open sub-pane borrows the MACD geometry, so the
			// fallback's KDJ rect is empty even when KDJ is the band
			// being drawn. Fall back to the MACD height there.
			kdj := fallback.KDJ.H
			if kdj <= 0 {
				kdj = fallback.MACD.H
			}
			heights = append(heights, kdj)
		default:
			heights = append(heights, extraSubH(fallback, len(heights)))
		}
	}
	for len(heights) < subs {
		heights = append(heights, fallback.MACD.H)
	}
	if h.Sub > 0 {
		for i := range heights {
			heights[i] = float32(h.Sub)
		}
	}
	total := volH
	for _, one := range heights {
		total += one
	}
	priceH := inner - total
	if priceH < inner*0.25 {
		// Shrink the bands to leave the price plot a quarter of the body.
		priceH = inner * 0.25
		avail := inner - priceH
		if total > 0 && avail > 0 {
			k := avail / total
			volH *= k
			for i := range heights {
				heights[i] *= k
			}
		}
	}
	out := Panes{}
	out.Price = ui.Rect{X: r.X, Y: r.Y, W: fallback.Price.W, H: priceH}
	out.PriceAxis = ui.Rect{X: fallback.PriceAxis.X, Y: r.Y, W: fallback.PriceAxis.W, H: priceH}
	y := r.Y + priceH + paneGap
	out.Volume = ui.Rect{X: r.X, Y: y, W: fallback.Price.W, H: volH}
	out.VolumeAxis = ui.Rect{X: fallback.VolumeAxis.X, Y: y, W: fallback.VolumeAxis.W, H: volH}
	y += volH + paneGap
	axisX := r.X + fallback.Price.W
	// Placement follows the same band order as the heights above, so the
	// KDJ band lands on KDJ even when the built-in split moved it there
	// from the MACD slot.
	for i, height := range heights {
		rect := ui.Rect{X: r.X, Y: y, W: fallback.Price.W, H: height}
		ax := ui.Rect{X: axisX, Y: y, W: fallback.PriceAxis.W, H: height}
		switch {
		case i < len(order) && order[i] == bandMACD:
			out.MACD, out.MACDAxis = rect, ax
		case i < len(order) && order[i] == bandKDJ:
			out.KDJ, out.KDJAxis = rect, ax
		default:
			out.Extra = append(out.Extra, rect)
			out.ExtraAxis = append(out.ExtraAxis, ax)
		}
		y += height + paneGap
	}
	// The time strip sits at the bottom of the body, one gap below the
	// last band. It is measured from the body rather than from the
	// running y so rounding in the band heights cannot push it out of
	// the box and make fitPanes shrink it.
	if fallback.Time.H > 0 {
		out.Time = ui.Rect{
			X: fallback.Time.X,
			Y: r.Y + bodyH + paneGap,
			W: fallback.Time.W,
			H: fallback.Time.H,
		}
	}
	return fitPanes(r, out)
}

// extraSubH is the built-in height of extra band i, for the case where
// the host set no Sub and the fallback split still has a rect for it.
func extraSubH(fallback Panes, i int) float32 {
	if i >= 0 && i < len(fallback.Extra) {
		return fallback.Extra[i].H
	}
	return fallback.MACD.H
}

// subBand is the band one open sub-pane indicator paints into.
type subBand int

const (
	// bandNone marks an indicator that is not a sub-pane.
	bandNone subBand = iota
	// bandMACD is the built-in MACD band, used by the first *MACD.
	bandMACD
	// bandKDJ is the built-in KDJ band, used by the first *KDJ.
	bandKDJ
	// bandExtra is an extra band under the built-ins, in indicator order.
	bandExtra
)

// subBands gives every indicator in inds the band it paints into, in the
// same order. The first *MACD takes the built-in MACD band, the first
// *KDJ the KDJ band, and every other sub-pane takes the next extra band.
// Layout, paint, and the theme colors all read this one order, so a band
// cannot be laid out without being painted, or painted with another
// band's colors.
func subBands(inds []Indicator) []subBand {
	out := make([]subBand, len(inds))
	macdUsed, kdjUsed := false, false
	for i, ind := range inds {
		if ind == nil || ind.PaneKind() != PaneSub {
			continue
		}
		switch ind.(type) {
		case *MACD:
			if !macdUsed {
				macdUsed = true
				out[i] = bandMACD
				continue
			}
		case *KDJ:
			if !kdjUsed {
				kdjUsed = true
				out[i] = bandKDJ
				continue
			}
		}
		out[i] = bandExtra
	}
	return out
}

func assignSubRects(p Panes, subs []Indicator, n int) Panes {
	plots, axes := takeSubRects(p, n)
	p.MACD, p.MACDAxis = ui.Rect{}, ui.Rect{}
	p.KDJ, p.KDJAxis = ui.Rect{}, ui.Rect{}
	p.Extra, p.ExtraAxis = nil, nil
	for i, band := range subBands(subs) {
		if i >= len(plots) || i >= len(axes) {
			break
		}
		switch band {
		case bandMACD:
			p.MACD, p.MACDAxis = plots[i], axes[i]
		case bandKDJ:
			p.KDJ, p.KDJAxis = plots[i], axes[i]
		default:
			p.Extra = append(p.Extra, plots[i])
			p.ExtraAxis = append(p.ExtraAxis, axes[i])
		}
	}
	return p
}

func takeSubRects(p Panes, n int) (plots, axes []ui.Rect) {
	switch {
	case n > 2:
		return append([]ui.Rect(nil), p.Extra...), append([]ui.Rect(nil), p.ExtraAxis...)
	case n == 2:
		return []ui.Rect{p.MACD, p.KDJ}, []ui.Rect{p.MACDAxis, p.KDJAxis}
	case n == 1:
		return []ui.Rect{p.MACD}, []ui.Rect{p.MACDAxis}
	default:
		return nil, nil
	}
}

// orderedSpecs keeps the toolbar in a stable order: the indicators that
// shipped first, then BOLL and KDJ. Registry order follows file name,
// which would put the new names first.
func orderedSpecs(in []Spec) []Spec {
	rank := func(name string) int {
		switch name {
		case "MA":
			return 0
		case "MACD":
			return 1
		case "BOLL":
			return 2
		case "KDJ":
			return 3
		default:
			return 100
		}
	}
	out := append([]Spec(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		return rank(out[i].Name) < rank(out[j].Name)
	})
	return out
}

func (a *Chart) bars() int {
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

// View paints the chart: one header per symbol, then the stack or the
// overlay plot, with volume and the indicators that are on. Pan, zoom,
// and the crosshair are on the plot. It does not paint a toolbar; the
// host binds those controls to SetPeriod, SetLayout, and the other setters.
//
// A window can use it directly:
//
//	ui.View(chart.View)
//
// A host that draws its own chrome calls chart.View(ctx) inside that view.
// While a replay is playing, View reveals the bars that are due and asks
// for another frame. Playback does not start a goroutine.
func (a *Chart) View(c *ui.Context) {
	// Playback is driven from the frame so it shares the UI goroutine
	// with paint. Invalidate asks for the next frame while it is running.
	if a.replay != nil {
		a.replay.Tick()
		if a.replay.Playing() {
			c.Invalidate()
		}
	}
	t := c.Theme()
	if len(a.panes) == 0 {
		// The symbol list was refused; LoadError says why.
		ui.Column(c).Fill().Background(t.Background)
		return
	}
	// Load from the offset the previous frame left behind, before headers
	// read the series, so a prepend and its offset shift land together.
	a.ensureForward()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// Headers are built before the charts report the pointer, so they
		// show crossIndex from the previous frame. The charts record the
		// new index, and a changed index asks for one more frame so every
		// header catches up before the next input.
		// A crosshair the host placed by time starts as this frame's
		// answer, so a chart without a pointer of its own still paints
		// the cursor its linked chart sent. The pointer overwrites it
		// below when it is over this chart.
		nextIndex := -1
		nextPane := -1
		if a.crossFixed {
			nextIndex, nextPane = a.crossIndex, a.crossPane
		}
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
			// The key stays the symbol, which is unique and stable, so a
			// renamed display name does not rebuild the element. The
			// label is what a screen reader announces: a blind session
			// passes a neutral name and the ticker never reaches it.
			slots[i] = chartSlot{key: s.name, label: a.paneLabel(i), pane: i, showTime: i == len(a.panes)-1}
		}
		if a.layout == layoutOverlay {
			// One plot. Both headers stay, so the crosshair can show
			// each symbol's own values at the shared bar index. A third
			// symbol is not drawn here, so it has no header either.
			if !a.hideHeader {
				for i := 0; i < len(a.panes) && i < 2; i++ {
					a.symbolHeader(c, i)
				}
			}
			slots = []chartSlot{{key: "compare", label: "Compare chart", pane: 0, showTime: true}}
		}
		for _, slot := range slots {
			if a.layout != layoutOverlay && !a.hideHeader {
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
				inPlot := pane >= 0 && pane < len(a.painted) && x >= 0 && x <= a.painted[pane].Price.W
				if !inPlot {
					// A pointer over the price axis has no bar to report.
					// Clear the pane as well, or paintPane still draws its
					// horizontal line and price tag from the stale pane.
					nextPane = -1
				} else if bar, ok := a.vp.IndexAt(float64(x), a.bars()); ok {
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
			// A host-placed crosshair survives a frame with no pointer:
			// that crosshair exists to mirror another chart, and the
			// pointer being elsewhere is exactly when it matters. Without
			// one, nothing is hovered.
			if !a.crossFixed {
				nextIndex = -1
				nextPane = -1
			}
		} else if a.crossFixed {
			// The pointer moved on this chart, so it takes the crosshair
			// back from the host's placed one.
			a.crossAtSet = false
			a.crossFixed = false
		}
		changed := nextIndex != a.crossIndex || nextPane != a.crossPane
		if changed || hoverX != a.hoverX || hoverY != a.hoverY {
			a.crossIndex = nextIndex
			a.crossPane = nextPane
			a.hoverX, a.hoverY = hoverX, hoverY
			c.AnimationFrame()
		}
		if changed && a.crossHook != nil {
			sym := ""
			if nextPane >= 0 && nextPane < len(a.panes) {
				sym = a.panes[nextPane].name
			}
			a.crossHook(sym, nextIndex, nextIndex >= 0)
		}
	})
}

// applyPriceDigits tells an indicator whose values are prices which
// precision the chart prints at. An indicator that is not price
// dimensioned has no such method and keeps its own formatting, so KDJ's
// 90.91 does not become 91 when a panel asks for whole-number prices.
func applyPriceDigits(ind Indicator, digits int) {
	if ps, ok := ind.(PriceScale); ok {
		ps.SetPriceDigits(digits)
	}
}

// displayName is the name a host wants shown for pane i, or the symbol
// when it gave none.
func (a *Chart) displayName(i int) string {
	if i >= 0 && i < len(a.displayNames) && a.displayNames[i] != "" {
		return a.displayNames[i]
	}
	if i >= 0 && i < len(a.panes) {
		return a.panes[i].name
	}
	return ""
}

// paneLabel is the accessible name of pane i's element. The symbol is
// what a sighted host means by its chart, so the label says "chart"; a
// screen reader then announces the display name next to it.
func (a *Chart) paneLabel(i int) string {
	name := a.displayName(i)
	if name == "" {
		return ""
	}
	return name + " chart"
}

func (a *Chart) symbolHeader(c *ui.Context, i int) {
	if i < 0 || i >= len(a.panes) {
		return
	}
	t := c.Theme()
	ui.Row(c).Padding(2, 14, 2, 14).Gap(10).AlignItems(ui.Center).Children(func() {
		name := ui.Text(c, a.displayName(i)).Bold().FontSize(13)
		if a.layout == layoutOverlay && i == 1 {
			name.TextColor(a.chartStyle(t).Compare)
		}
		ui.Text(c, a.readout(i)).Font("monospace").FontSize(12).NoWrap().Grow(1)
	})
}

func (a *Chart) paintPane(p *ui.Painter, r ui.Rect, t *ui.Theme, i int, showTime bool) {
	a.rememberFrame(i, r, showTime)
	panes := a.layoutPanes(r, showTime)
	if i >= len(a.painted) {
		next := make([]Panes, i+1)
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
	// A crosshair the host placed by time has no pointer of its own: it
	// mirrors another chart's pointer, and that pointer's coordinates
	// belong to the other chart's layout. Building a Pointer from
	// a.hoverX/hoverY here painted a horizontal line and a price tag at
	// whatever this chart's stale hover position happened to be (0,0 on
	// a linked chart), which put the tag at the top of the plot with a
	// price that was not a price of this series. The vertical line still
	// draws, because it comes from crossIndex.
	ptr := Pointer{}
	if a.crossPane == i && !a.crossFixed {
		ptr = Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	s := &a.panes[i]
	style := a.chartStyle(t)
	active := a.active(s)
	paint := barPaint{style: style, host: a.barColor}
	scale, _ := drawPriceHook(p, panes, s.candles, active, a.vp, ptr, a.crossIndex, paint, a.labels(), a.subDraw)
	var rubber *Rubber
	if a.draftOn && a.draftPane == i && a.crossPane == i {
		rubber = &Rubber{From: a.draft, X: float64(a.hoverX), Y: float64(a.hoverY), Kind: a.draftRubberKind()}
	}
	DrawOverlays(p, panes.Price, s.overlays.live(), a.vp, scale, s.candles, style, rubber)
	a.drawMarkers(p, panes.Price, i, s.candles, a.vp, scale, style, 1, false)
	a.applyDeferred(i)
}

// paintCompare draws both symbols in one column. Volume, MACD, KDJ, and
// drawings belong to the primary symbol. The second symbol is the
// close line painted by PaintCompare.
func (a *Chart) paintCompare(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	a.rememberFrame(0, r, true)
	panes := a.layoutPanes(r, true)
	a.painted = []Panes{panes}
	a.plotW = float64(panes.Price.W)
	if !a.fitted && panes.Price.W > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
	a.vp.Clamp(a.bars(), a.plotW)
	ptr := Pointer{}
	if a.crossPane == 0 {
		ptr = Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	primary, compare := a.overlayPair()
	style := a.chartStyle(t)
	active := a.active(&a.panes[0])
	paint := barPaint{style: style, host: a.barColor}
	scale, indexed := drawCompare(p, panes, primary, compare, active, a.vp, ptr, a.crossIndex, paint, a.labels())
	var rubber *Rubber
	if a.draftOn && a.draftPane == 0 && a.crossPane == 0 {
		rubber = &Rubber{From: a.draft, X: float64(a.hoverX), Y: float64(a.hoverY), Kind: a.draftRubberKind()}
	}
	DrawOverlays(p, panes.Price, a.panes[0].overlays.live(), a.vp, scale, primary, style, rubber)
	a.drawMarkers(p, panes.Price, 0, primary, a.vp, scale, style, 1, false)
	if indexed && len(a.panes) > 1 {
		// Compare prices are indexed to the primary base close. The
		// marker scale is the raw scale, so convert by that ratio. The
		// compare series is drawn as a close line, so its arrows anchor
		// on Close rather than the unplotted high or low.
		base := a.compareBase()
		compare := a.panes[1].candles
		if indexable(primary, base) && indexable(compare, base) {
			a.drawMarkers(p, panes.Price, 1, compare, a.vp, scale, style, primary[base].Close/compare[base].Close, true)
		}
	}
	a.applyDeferred(0)
}

// chartStyle is the window theme's chart colors, or the style passed to
// NewChart, with the hollow-candle switch applied. Paint paths take this
// instead of literal colors.
func (a *Chart) chartStyle(t *ui.Theme) Style {
	s := StyleFrom(t)
	if a.styleSet {
		s = a.style
	}
	s.HollowUp = a.hollowUp
	s.LimitPct = a.limitPct
	return s
}

func (a *Chart) readout(i int) string {
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
	labels := a.labels()
	line := label + "  " + formatBar(s.candles[idx], math.NaN(), math.NaN(), labels)
	if note := markerReadout(s.marks, idx, s.candles[idx], labels); note != "" {
		line += "  " + note
	}
	if a.layout == layoutOverlay {
		// Symbols after the second are not on the overlay plot.
		if i > 1 {
			return line
		}
		if indexed := a.overlayIndexed(); indexed {
			line += "  idx " + labels.price(a.indexValue(i, idx))
		}
		// The compare series is a close line. Its MA and MACD are not
		// drawn, so they stay off this header.
		if i != 0 {
			return line
		}
	}
	for _, ind := range a.active(&a.panes[i]) {
		applyPriceDigits(ind, a.priceDigits)
		if v := ind.ValueAt(idx); v != "" {
			line += "  " + v
		}
	}
	return line
}

// indexValue is close[idx] / close[base] * 100 for one symbol. base is
// the bar PaintCompare indexes to 100: the first visible bar where both
// overlay series have a positive close.
func (a *Chart) indexValue(pane, idx int) float64 {
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

func (a *Chart) compareBase() int {
	primary, compare := a.overlayPair()
	base, _, _ := overlayBase(primary, compare, a.vp, a.plotW)
	return base
}

func (a *Chart) overlayIndexed() bool {
	primary, compare := a.overlayPair()
	base := a.compareBase()
	return indexableOverlay(primary, compare, base)
}
