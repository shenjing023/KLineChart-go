package kline

import (
	"fmt"
	"math"
	"time"
)

// liveSub is one SubscribeBar the window currently holds.
type liveSub struct {
	symbol string
	period Period
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
			// Fence handlers before calling UnsubscribeBar. A feed may
			// invoke a still-held handler from its unsubscribe path.
			a.liveGen.Add(1)
			for _, sub := range subs {
				_ = live.UnsubscribeBar(sub.symbol, sub.period)
			}
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

// maxHeldBars caps how many bars one symbol may hold while it waits for
// the others to reach the same open time. A feed that stops, or one that
// runs far ahead, would otherwise grow the pending list for the life of
// the chart. The newest bars are kept, because those are the ones the
// other symbols can still match; the oldest are dropped and reported
// through AlignError.
const maxHeldBars = 512

// hold appends bar to s's pending list and enforces maxHeldBars. It
// reports whether any older pending bar had to be dropped.
func (a *Chart) hold(s *series, bar Candle) bool {
	s.held = append(s.held, bar)
	return trimHeld(a, s)
}

// trimHeld keeps the newest pending window by moving the retained
// tail to the front of the reusable slice. Initial loads and live
// appends share the cap arithmetic.
func trimHeld(a *Chart, s *series) bool {
	if len(s.held) <= maxHeldBars {
		return false
	}
	drop := len(s.held) - maxHeldBars
	oldest := s.held[drop-1]
	s.held = append(s.held[:0], s.held[drop:]...)
	if a != nil {
		a.alignErr = heldDropError(s.name, drop, oldest)
	}
	return true
}

func heldDropError(name string, drop int, oldest Candle) error {
	return fmt.Errorf("%w: %s dropped %d held bar(s) up to %s, the pending window is %d",
		ErrMisaligned, name, drop, oldest.Time.UTC().Format("2006-01-02 15:04"), maxHeldBars)
}

// applyLiveBar takes one bar for symbol. The symbol's newest open time,
// held or shown, is revised in place. A newer open time is held until
// every symbol has a bar at that open time, then those bars are appended
// together, so candle i stays the same open time on every pane. An older
// open time is ignored. Shown candles update indicators from that bar
// when the indicator implements BarTail, and recalculate otherwise. A
// view sitting on the right edge stays there; a panned view does not jump.
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
			prev := s.candles[n-1]
			s.candles[n-1] = bar
			a.reviseInds(s, prev)
			return
		case !bar.Time.After(s.candles[n-1].Time):
			return
		}
	}
	before := a.bars()
	pinned := a.pinnedRight()
	a.hold(s, bar)
	if !a.promote() {
		return
	}
	a.afterPromote(before, pinned)
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
