package kline

import (
	"fmt"
	"math"
	"sort"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
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
	// style, when styleSet, replaces theme colors. The live hollow
	// switch is hollowUp, not Style.HollowUp. limitPct is the live
	// limit threshold Paint reads, including when the colors still
	// come from the theme.
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
	// symbolErr is set when NewChart refused the symbol list. The chart
	// then has no panes, and every load returns it.
	symbolErr error
	// priceDigits is the Readout and crosshair precision. digitsSet
	// distinguishes a whole-number precision of 0 from a zero Chart,
	// which still prints two decimals.
	priceDigits int
	digitsSet   bool
	// zone formats bar times in the header and the crosshair tag. Nil is UTC.
	zone *time.Location
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
	return labelFmt{digits: a.priceDigits, loc: a.zone}
}

// liveSub is one SubscribeBar the window currently holds.
type liveSub struct {
	symbol string
	period Period
}

// setPeriod reloads every pane with Kind=init from the current source.
// An invalid p is rejected before the source is asked. A failed GetBars,
// or windows that cannot share one bar index, leave the bars, the period,
// and the subscription as they were.
func (a *Chart) setPeriod(p Period) error {
	if a.symbolErr != nil {
		return a.symbolErr
	}
	if !p.Valid() {
		return ErrInvalidPeriod
	}
	next, err := a.fetchInit(a.src, p)
	if err != nil {
		return err
	}
	return a.install(p, next)
}

// fetchInit loads the latest window of every symbol at p from src. It
// does not change the chart. Each batch is copied as it arrives, so a
// source may reuse the slice it returned.
func (a *Chart) fetchInit(src DataSource, p Period) ([]series, error) {
	if src == nil {
		return nil, ErrNilSource
	}
	next := make([]series, len(a.panes))
	for i, s := range a.panes {
		bars, more, err := src.GetBars(BarRequest{
			Symbol: s.name,
			Period: p,
			Kind:   KindInit,
			Limit:  sampleBars,
		})
		if err != nil {
			return nil, err
		}
		next[i] = series{name: s.name, candles: cloneCandles(bars), more: more}
	}
	if err := alignInit(next); err != nil {
		return nil, err
	}
	return next, nil
}

// alignInit puts every symbol's init window on one bar index. Windows
// start at the latest first open time; earlier bars are dropped, and that
// symbol keeps more set because older bars exist. Bars after the earliest
// last open time are held until the other symbols reach them, which is
// the usual case when one feed has opened the next bar first. What is
// left must match open time for open time. A symbol with no bars next to
// one that has bars cannot share an index.
func alignInit(next []series) error {
	if len(next) < 2 {
		return nil
	}
	empty, full := -1, -1
	var first, last time.Time
	for i, s := range next {
		if len(s.candles) == 0 {
			if empty < 0 {
				empty = i
			}
			continue
		}
		head, tail := s.candles[0].Time, s.candles[len(s.candles)-1].Time
		if full < 0 || head.After(first) {
			first = head
		}
		if full < 0 || tail.Before(last) {
			last = tail
		}
		full = i
	}
	switch {
	case full < 0:
		return nil
	case empty >= 0:
		return fmt.Errorf("%w: %s has no bars and %s does", ErrMisaligned, next[empty].name, next[full].name)
	case first.After(last):
		return fmt.Errorf("%w: the windows share no open time", ErrMisaligned)
	}
	lo := make([]int, len(next))
	hi := make([]int, len(next))
	for i, s := range next {
		l, h := 0, len(s.candles)
		for l < h && s.candles[l].Time.Before(first) {
			l++
		}
		for h > l && s.candles[h-1].Time.After(last) {
			h--
		}
		lo[i], hi[i] = l, h
	}
	ref := next[0].candles[lo[0]:hi[0]]
	for i := 1; i < len(next); i++ {
		if at, ok := firstTimeDiff(ref, next[i].candles[lo[i]:hi[i]]); ok {
			return fmt.Errorf("%w: %s and %s differ at %s", ErrMisaligned, next[0].name, next[i].name, at.UTC().Format("2006-01-02 15:04"))
		}
	}
	for i := range next {
		s := &next[i]
		if lo[i] > 0 {
			s.more = true
		}
		if hi[i] < len(s.candles) {
			s.held = append([]Candle(nil), s.candles[hi[i]:]...)
		}
		// Capped so an append to candles cannot write over held.
		s.candles = s.candles[lo[i]:hi[i]:hi[i]]
	}
	return nil
}

// firstTimeDiff is the earliest open time at which a and b disagree,
// including a bar one of them has and the other does not.
func firstTimeDiff(a, b []Candle) (time.Time, bool) {
	n := min(len(a), len(b))
	for j := 0; j < n; j++ {
		if !a[j].Time.Equal(b[j].Time) {
			if b[j].Time.Before(a[j].Time) {
				return b[j].Time, true
			}
			return a[j].Time, true
		}
	}
	switch {
	case len(a) > n:
		return a[n].Time, true
	case len(b) > n:
		return b[n].Time, true
	}
	return time.Time{}, false
}

// install replaces the panes with a fresh load and fits the latest bars
// when the plot width is already known. The first frame still fits from
// paint, because layout has not run yet. Drawings, the crosshair, and a
// drawing or drag in progress belong to the old bars and are dropped.
// The live subscription follows the new period and source.
func (a *Chart) install(p Period, next []series) error {
	for i := range next {
		next[i] = a.mount(next[i], next[i].candles, next[i].more)
	}
	a.period = p
	a.panes = next
	a.crossIndex = -1
	a.crossPane = -1
	a.cancelGesture()
	a.fitted = false
	if a.plotW > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
	a.clearForwardHold()
	a.alignErr = nil
	a.loadErr = a.followLive()
	return a.loadErr
}

// cancelGesture drops a segment draft, an overlay drag, and a pan-tool
// click. Each refers to bars or pixels of the panes it started on.
func (a *Chart) cancelGesture() {
	a.draftOn = false
	a.dragOn = false
	a.dragNewHLine = false
	a.pressOn = false
}

// followLive subscribes when src implements LiveSource and drops any
// previous subscription, including one on a source the window no longer
// uses. A plain DataSource, such as SyntheticSource, is ignored. Nothing
// here opens a connection. A SubscribeBar error unsubscribes the symbols
// that already succeeded and fences their handlers.
func (a *Chart) followLive() error {
	_ = a.dropLive()
	live, ok := a.src.(LiveSource)
	if !ok || live == nil {
		return nil
	}
	gen := a.liveGen.Load()
	subs := make([]liveSub, 0, len(a.panes))
	for _, s := range a.panes {
		symbol := s.name
		period := a.period
		err := live.SubscribeBar(symbol, period, func(bar Candle) {
			a.onLiveBar(gen, symbol, period, bar)
		})
		if err != nil {
			for _, sub := range subs {
				_ = live.UnsubscribeBar(sub.symbol, sub.period)
			}
			a.liveGen.Add(1)
			return err
		}
		subs = append(subs, liveSub{symbol: symbol, period: period})
	}
	if len(subs) == 0 {
		return nil
	}
	a.live = live
	a.liveSubs = subs
	return nil
}

func (a *Chart) dropLive() error {
	a.liveGen.Add(1)
	live := a.live
	subs := a.liveSubs
	a.live = nil
	a.liveSubs = nil
	if live == nil {
		return nil
	}
	var err error
	for _, sub := range subs {
		if uerr := live.UnsubscribeBar(sub.symbol, sub.period); uerr != nil && err == nil {
			err = uerr
		}
	}
	return err
}

// onLiveBar applies bar when the subscription is still current. The
// generation check is safe from the feed's goroutine. Period, symbol,
// and the candle slice are touched only inside apply, which Marshal
// runs on the UI thread. The handler must not call SetPeriod, SetSource,
// or View.
func (a *Chart) onLiveBar(gen uint64, symbol string, period Period, bar Candle) {
	if gen != a.liveGen.Load() {
		return
	}
	apply := func() {
		if gen != a.liveGen.Load() || period != a.period || a.paneOf(symbol) < 0 {
			return
		}
		a.applyLiveBar(symbol, bar)
	}
	if a.marshal != nil {
		a.marshal(apply)
		return
	}
	apply()
}

func (a *Chart) paneOf(symbol string) int {
	for i := range a.panes {
		if a.panes[i].name == symbol {
			return i
		}
	}
	return -1
}

// applyLiveBar takes one bar for symbol. The symbol's newest open time,
// held or shown, is revised in place. A newer open time is held until
// every symbol has a bar at that open time, then those bars are appended
// together, so candle i stays the same open time on every pane. An older
// open time is ignored. Indicators are recalculated when shown candles
// change. A view sitting on the right edge stays there; a panned view
// does not jump.
func (a *Chart) applyLiveBar(symbol string, bar Candle) {
	i := a.paneOf(symbol)
	if i < 0 {
		return
	}
	s := &a.panes[i]
	if h := len(s.held); h > 0 {
		switch {
		case bar.Time.Equal(s.held[h-1].Time):
			s.held[h-1] = bar
			return
		case !bar.Time.After(s.held[h-1].Time):
			return
		}
	} else if n := len(s.candles); n > 0 {
		switch {
		case bar.Time.Equal(s.candles[n-1].Time):
			s.candles[n-1] = bar
			a.recalc(s)
			return
		case !bar.Time.After(s.candles[n-1].Time):
			return
		}
	}
	s.held = append(s.held, bar)
	pinned := a.pinnedRight()
	if !a.promote() {
		return
	}
	for j := range a.panes {
		a.recalc(&a.panes[j])
	}
	if pinned && a.plotW > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
	}
}

// promote appends held bars once every symbol has one at the same open
// time. A held bar older than another symbol's oldest held bar can never
// be matched, because that symbol has moved past its open time; it is
// dropped, and AlignError reports it. promote reports whether any bar was
// appended.
func (a *Chart) promote() bool {
	moved := false
	for len(a.panes) > 0 {
		var front time.Time
		for i := range a.panes {
			held := a.panes[i].held
			if len(held) == 0 {
				return moved
			}
			if i == 0 || held[0].Time.After(front) {
				front = held[0].Time
			}
		}
		matched := true
		for i := range a.panes {
			s := &a.panes[i]
			drop := 0
			for drop < len(s.held) && s.held[drop].Time.Before(front) {
				drop++
			}
			if drop > 0 {
				a.alignErr = fmt.Errorf("%w: %s live bar at %s dropped, another symbol skipped that open time",
					ErrMisaligned, s.name, s.held[drop-1].Time.UTC().Format("2006-01-02 15:04"))
				s.held = s.held[drop:]
			}
			if len(s.held) == 0 || !s.held[0].Time.Equal(front) {
				matched = false
			}
		}
		if !matched {
			continue
		}
		for i := range a.panes {
			s := &a.panes[i]
			s.candles = append(s.candles, s.held[0])
			s.held = s.held[1:]
		}
		moved = true
	}
	return moved
}

func (a *Chart) pinnedRight() bool {
	if !a.fitted || a.plotW <= 1 || a.bars() <= 0 {
		return false
	}
	v := a.vp
	v.FitRight(a.bars(), a.plotW)
	return math.Abs(a.vp.Offset-v.Offset) < 1e-6
}

// ensureForward prepends older bars when the shared offset is near the
// left edge. The prepend is one transaction: every pane that currently
// has bars must accept the same count of strictly-earlier bars, with the
// same open times. That count is the trailing run every batch shares;
// bars before the first open time the symbols disagree on stay out.
// Offset then increases by that count, which is the same rule as
// Viewport: bar i moves to i+n, so (i+n) - (offset+n) is unchanged. A
// pane that cannot accept the batch leaves offset and crossIndex where
// they are, and a batch with no shared tail is reported by AlignError.
// more is stored as the source reported it, or set when a batch had
// bars past the shared tail. A GetBars error changes nothing and waits
// forwardBackoff before the next try, so a frame loop does not call the
// source again immediately. A load already in flight does not start
// another.
func (a *Chart) ensureForward() {
	if a.loading || a.src == nil || !a.fitted || a.vp.Offset >= forwardEdge || a.forwardCooling() || !a.canForward() {
		return
	}
	a.loading = true
	defer func() { a.loading = false }()

	got := make([]forwardFetch, len(a.panes))
	for i, s := range a.panes {
		if len(s.candles) == 0 {
			continue
		}
		bars, more, err := a.src.GetBars(BarRequest{
			Symbol: s.name,
			Period: a.period,
			Kind:   KindForward,
			Anchor: s.candles[0].Time,
			Limit:  forwardBatch,
		})
		if err != nil {
			a.holdForward()
			return
		}
		// Copy before the next symbol's request: a source may reuse
		// one buffer for every GetBars.
		bars = cloneCandles(strictlyBefore(bars, s.candles[0].Time))
		got[i] = forwardFetch{bars: bars, more: more, ok: true}
	}
	for i, s := range a.panes {
		if len(s.candles) == 0 {
			continue
		}
		if !got[i].ok || len(got[i].bars) == 0 {
			a.keepReportedMore(got)
			return
		}
	}
	n := sharedTail(a.panes, got)
	if n == 0 {
		a.alignErr = fmt.Errorf("%w: older bars end on different open times", ErrMisaligned)
		a.keepReportedMore(got)
		return
	}
	for i := range a.panes {
		if len(a.panes[i].candles) == 0 {
			continue
		}
		older := got[i].bars
		more := got[i].more || len(older) > n
		a.panes[i] = a.mount(a.panes[i], prependBars(older[len(older)-n:], a.panes[i].candles), more)
	}
	a.vp.Offset += float64(n)
	if a.crossIndex >= 0 {
		a.crossIndex += n
	}
	a.clearForwardHold()
}

// canForward reports whether every pane that has bars can take a
// strictly-earlier batch. One pane with more set false blocks the load:
// shifting offset for the others would break the shared open time.
func (a *Chart) canForward() bool {
	ready := false
	for _, s := range a.panes {
		if len(s.candles) == 0 {
			continue
		}
		if !s.more {
			return false
		}
		ready = true
	}
	return ready
}

// forwardFetch is one pane's forward response. ok is false when that
// pane was not asked, which is a pane that currently has no bars.
type forwardFetch struct {
	bars []Candle
	more bool
	ok   bool
}

// sharedTail is how many trailing bars every fetched batch has in common,
// open time for open time. Each batch ends just before its pane's first
// bar, and those first bars match, so the shared tail lands on the same
// indexes on every pane.
func sharedTail(panes []series, got []forwardFetch) int {
	n := -1
	var ref []Candle
	for i := range panes {
		if i >= len(got) || !got[i].ok {
			continue
		}
		b := got[i].bars
		if n < 0 {
			ref, n = b, len(b)
			continue
		}
		m := 0
		for m < n && m < len(b) && b[len(b)-1-m].Time.Equal(ref[len(ref)-1-m].Time) {
			m++
		}
		n = m
	}
	return max(n, 0)
}

// keepReportedMore stores each fetched more flag and, when every pane
// with bars still says more, waits before asking again. An empty
// strictly-earlier batch with more still true would otherwise run on
// every frame.
func (a *Chart) keepReportedMore(got []forwardFetch) {
	for i := range a.panes {
		if i < len(got) && got[i].ok {
			a.panes[i].more = got[i].more
		}
	}
	if a.canForward() {
		a.holdForward()
	}
}

func (a *Chart) forwardCooling() bool {
	return !a.forwardHold.IsZero() && time.Now().Before(a.forwardHold)
}

func (a *Chart) holdForward() {
	a.forwardHold = time.Now().Add(forwardBackoff)
}

func (a *Chart) clearForwardHold() {
	a.forwardHold = time.Time{}
}

func strictlyBefore(bars []Candle, anchor time.Time) []Candle {
	end := len(bars)
	for end > 0 && !bars[end-1].Time.Before(anchor) {
		end--
	}
	return bars[:end]
}

func prependBars(older, newer []Candle) []Candle {
	out := make([]Candle, 0, len(older)+len(newer))
	out = append(out, older...)
	out = append(out, newer...)
	return out
}

// mount stores bars on s and recalculates the indicators that are on.
// Existing instances are kept so a prepend does not allocate a new MA
// or MACD, only a new value slice inside Calc.
func (a *Chart) mount(s series, bars []Candle, more bool) series {
	s.candles = bars
	s.more = more
	a.recalc(&s)
	return s
}

// recalc runs Calc for s's indicators that are on, after its candles
// changed. The others are marked stale, and active calculates one when
// it is turned on.
func (a *Chart) recalc(s *series) {
	a.ensureInds(s)
	for i, ind := range s.inds {
		s.fresh[i] = false
		if ind != nil && i < len(a.slots) && a.slots[i].on {
			ind.Calc(s.candles)
			s.fresh[i] = true
		}
	}
}

// ensureInds gives s one indicator instance per slot, in slot order.
func (a *Chart) ensureInds(s *series) {
	for len(s.inds) < len(a.slots) {
		s.inds = append(s.inds, a.slots[len(s.inds)].spec.New())
	}
	for len(s.fresh) < len(s.inds) {
		s.fresh = append(s.fresh, false)
	}
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
	switch n {
	case 0:
		// SplitStack leaves a zero-height MACD rect. Keep it so this
		// layout stays identical to the built-in no-sub-pane split.
		return SplitStack(r, showTime, false, false)
	case 1:
		return assignSubRects(SplitStack(r, showTime, true, false), subs, n)
	case 2:
		return assignSubRects(SplitStack(r, showTime, true, true), subs, n)
	default:
		return assignSubRects(splitMany(r, showTime, n), subs, n)
	}
}

func assignSubRects(p Panes, subs []Indicator, n int) Panes {
	plots, axes := takeSubRects(p, n)
	p.MACD, p.MACDAxis = ui.Rect{}, ui.Rect{}
	p.KDJ, p.KDJAxis = ui.Rect{}, ui.Rect{}
	p.Extra, p.ExtraAxis = nil, nil
	for i, ind := range subs {
		if i >= len(plots) || i >= len(axes) {
			break
		}
		switch ind.(type) {
		case *MACD:
			p.MACD, p.MACDAxis = plots[i], axes[i]
		case *KDJ:
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
func (a *Chart) View(c *ui.Context) {
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

func (a *Chart) symbolHeader(c *ui.Context, i int) {
	if i < 0 || i >= len(a.panes) {
		return
	}
	s := &a.panes[i]
	t := c.Theme()
	ui.Row(c).Padding(2, 14, 2, 14).Gap(10).AlignItems(ui.Center).Children(func() {
		name := ui.Text(c, s.name).Bold().FontSize(13)
		if a.layout == layoutOverlay && i == 1 {
			name.TextColor(a.chartStyle(t).Compare)
		}
		ui.Text(c, a.readout(i)).Font("monospace").FontSize(12).NoWrap().Grow(1)
	})
}

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

// plotLocal converts a pointer event to price-plot DIPs. The price plot
// starts at the chart box's top-left, so the event's local axes match it.
// in is false over the volume strip, the sub-panes, and the axes.
func (a *Chart) plotLocal(pane int, ev ui.InputEvent) (x, y float64, in bool) {
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

func (a *Chart) paintPane(p *ui.Painter, r ui.Rect, t *ui.Theme, i int, showTime bool) {
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
	ptr := Pointer{}
	if a.crossPane == i {
		ptr = Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	s := &a.panes[i]
	style := a.chartStyle(t)
	active := a.active(s)
	scale := drawPrice(p, panes, s.candles, active, a.vp, ptr, a.crossIndex, style, a.labels())
	var rubber *Rubber
	if a.draftOn && a.draftPane == i && a.crossPane == i {
		rubber = &Rubber{From: a.draft, X: float64(a.hoverX), Y: float64(a.hoverY), Kind: a.draftRubberKind()}
	}
	DrawOverlays(p, panes.Price, s.overlays.live(), a.vp, scale, s.candles, style, rubber)
}

// paintCompare draws both symbols in one column. Volume, MACD, KDJ, and
// drawings belong to the primary symbol. The second symbol is the
// close line painted by PaintCompare.
func (a *Chart) paintCompare(p *ui.Painter, r ui.Rect, t *ui.Theme) {
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
	scale, _ := drawCompare(p, panes, primary, compare, active, a.vp, ptr, a.crossIndex, style, a.labels())
	var rubber *Rubber
	if a.draftOn && a.draftPane == 0 && a.crossPane == 0 {
		rubber = &Rubber{From: a.draft, X: float64(a.hoverX), Y: float64(a.hoverY), Kind: a.draftRubberKind()}
	}
	DrawOverlays(p, panes.Price, a.panes[0].overlays.live(), a.vp, scale, primary, style, rubber)
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
	if a.layout == layoutOverlay {
		// Symbols after the second are not on the overlay plot.
		if i > 1 {
			return line
		}
		line += "  idx " + labels.price(a.indexValue(i, idx))
		// The compare series is a close line. Its MA and MACD are not
		// drawn, so they stay off this header.
		if i != 0 {
			return line
		}
	}
	for _, ind := range a.active(&a.panes[i]) {
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
