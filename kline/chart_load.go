package kline

import (
	"fmt"
	"time"
)

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
