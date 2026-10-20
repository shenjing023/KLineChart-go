package kline

import (
	"errors"
	"fmt"
)

// ErrBarTime is returned when AppendBar or UpdateLast is given a time
// that is not the series edge. AppendBar needs an open time strictly
// after the last shown or held bar. UpdateLast needs the last open
// time, or a zero time, which keeps that open.
var ErrBarTime = errors.New("kline: bar time is not at the series edge")

// AppendBar holds bar until every symbol has a bar at the same open
// time, then appends those bars together. Indicators that implement
// BarTail update from the new close. The others are recalculated.
// A view sitting on the right edge follows the new bar. A panned view
// keeps its offset. An empty chart is treated as already at the edge,
// so the first bar is fitted once the plot has a width.
func (a *Chart) AppendBar(symbol string, bar Candle) error {
	if a == nil {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	i := a.paneOf(symbol)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	if bar.Time.IsZero() {
		return fmt.Errorf("%w: zero open time", ErrBarTime)
	}
	s := &a.panes[i]
	var last Candle
	switch {
	case len(s.held) > 0:
		last = s.held[len(s.held)-1]
	case len(s.candles) > 0:
		last = s.candles[len(s.candles)-1]
	}
	if !last.Time.IsZero() && !bar.Time.After(last.Time) {
		return fmt.Errorf("%w: %s is not after %s", ErrBarTime, bar.Time.UTC().Format("2006-01-02 15:04"), last.Time.UTC().Format("2006-01-02 15:04"))
	}
	before := a.bars()
	pinned := a.pinnedRight()
	s.held = append(s.held, bar)
	if !a.promote() {
		return nil
	}
	a.afterPromote(before, pinned)
	return nil
}

// UpdateLast revises the newest bar of symbol in place. A zero Time
// keeps the open time. Any other Time must be that open time. When the
// bar is already on screen, indicators update that one point. The
// viewport does not move: the series length is unchanged. A bar still
// held for the other symbols is revised there and does not touch the
// indicators yet.
func (a *Chart) UpdateLast(symbol string, bar Candle) error {
	if a == nil {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	i := a.paneOf(symbol)
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	s := &a.panes[i]
	if h := len(s.held); h > 0 {
		if !bar.Time.IsZero() && !bar.Time.Equal(s.held[h-1].Time) {
			return fmt.Errorf("%w: %s is not the held open", ErrBarTime, bar.Time.UTC().Format("2006-01-02 15:04"))
		}
		bar.Time = s.held[h-1].Time
		s.held[h-1] = bar
		return nil
	}
	n := len(s.candles)
	if n == 0 {
		return fmt.Errorf("%w: no bar to revise", ErrBarTime)
	}
	if !bar.Time.IsZero() && !bar.Time.Equal(s.candles[n-1].Time) {
		return fmt.Errorf("%w: %s is not the last open", ErrBarTime, bar.Time.UTC().Format("2006-01-02 15:04"))
	}
	prev := s.candles[n-1]
	bar.Time = prev.Time
	s.candles[n-1] = bar
	a.reviseInds(s, prev)
	return nil
}

// afterPromote updates indicators for bars promote just appended and
// follows the right edge when the view was already there.
func (a *Chart) afterPromote(before int, pinned bool) {
	gained := a.bars() - before
	for j := range a.panes {
		if gained == 1 {
			a.appendInds(&a.panes[j])
		} else {
			a.recalc(&a.panes[j])
		}
		a.reindexMarks(&a.panes[j])
	}
	a.followEdge(before, pinned)
}

// followEdge fits the latest bar when the view was on the right edge,
// or when there was nothing on screen to pan away from. A panned view
// is left where it was.
func (a *Chart) followEdge(before int, pinned bool) {
	if a.plotW <= 1 {
		return
	}
	if before == 0 || pinned {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
}

func (a *Chart) appendInds(s *series) {
	a.ensureInds(s)
	for i, ind := range s.inds {
		if ind == nil || i >= len(a.slots) || !a.slots[i].on {
			if i < len(s.fresh) {
				s.fresh[i] = false
			}
			continue
		}
		if s.fresh[i] {
			if tail, ok := ind.(BarTail); ok {
				tail.Append(s.candles)
				continue
			}
		}
		ind.Calc(s.candles)
		s.fresh[i] = true
	}
}

func (a *Chart) reviseInds(s *series, prev Candle) {
	a.ensureInds(s)
	for i, ind := range s.inds {
		if ind == nil || i >= len(a.slots) || !a.slots[i].on {
			if i < len(s.fresh) {
				s.fresh[i] = false
			}
			continue
		}
		if s.fresh[i] {
			if tail, ok := ind.(BarTail); ok {
				tail.Revise(s.candles, prev)
				continue
			}
		}
		ind.Calc(s.candles)
		s.fresh[i] = true
	}
}
