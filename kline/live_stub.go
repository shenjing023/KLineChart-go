package kline

import (
	"fmt"
	"sync"
)

// StubLiveSource is an in-memory LiveSource for tests and for wiring a
// chart to a feed that does not exist in this repo. SetBars installs
// history. Push delivers one bar to the handler subscribed for that
// symbol and period, and updates the stored series when the bar is new
// or revises the last open time. An older open time is still delivered
// and is not inserted into history. UnsubscribeBar stops later Pushes
// from calling the handler. There is no network connection.
type StubLiveSource struct {
	mu   sync.Mutex
	hist map[liveKey][]Candle
	subs map[liveKey]BarHandler
}

type liveKey struct {
	symbol string
	period Period
}

var _ LiveSource = (*StubLiveSource)(nil)

// SetBars replaces the stored history for symbol and period. The slice
// is copied. A subscriber is not called.
func (s *StubLiveSource) SetBars(symbol string, period Period, bars []Candle) error {
	if !period.Valid() {
		return ErrInvalidPeriod
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.hist[liveKey{symbol, period}] = cloneCandles(bars)
	return nil
}

// GetBars implements DataSource from the stored series. KindInit returns
// the latest Limit bars. KindForward returns bars strictly before Anchor.
// KindBackward is rejected.
func (s *StubLiveSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	if !req.Period.Valid() {
		return nil, false, ErrInvalidPeriod
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.hist[liveKey{req.Symbol, req.Period}]
	switch req.Kind {
	case KindInit, "":
		if len(all) <= limit {
			return cloneCandles(all), false, nil
		}
		return cloneCandles(all[len(all)-limit:]), true, nil
	case KindForward:
		if req.Anchor.IsZero() {
			return nil, false, fmt.Errorf("kline: forward request requires anchor")
		}
		n := 0
		for n < len(all) && all[n].Time.Before(req.Anchor) {
			n++
		}
		if n == 0 {
			return []Candle{}, false, nil
		}
		if n <= limit {
			return cloneCandles(all[:n]), false, nil
		}
		return cloneCandles(all[n-limit : n]), true, nil
	default:
		return nil, false, fmt.Errorf("%w: %q", ErrUnsupportedKind, req.Kind)
	}
}

// SubscribeBar registers onBar for symbol and period. A second call for
// the same pair replaces the handler. The handler runs on the goroutine
// that calls Push.
func (s *StubLiveSource) SubscribeBar(symbol string, period Period, onBar BarHandler) error {
	if onBar == nil {
		return ErrNilBarHandler
	}
	if !period.Valid() {
		return ErrInvalidPeriod
	}
	if s == nil {
		return fmt.Errorf("kline: nil live source")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.subs[liveKey{symbol, period}] = onBar
	return nil
}

// UnsubscribeBar drops the handler for symbol and period. A pair that
// was not subscribed returns nil.
func (s *StubLiveSource) UnsubscribeBar(symbol string, period Period) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs == nil {
		return nil
	}
	delete(s.subs, liveKey{symbol, period})
	return nil
}

// Push stores bar when it extends the series or revises the last open
// time, then calls the current handler. After UnsubscribeBar, Push does
// not call a handler. A bar older than the last open time is delivered
// and left out of the stored series.
func (s *StubLiveSource) Push(symbol string, period Period, bar Candle) {
	if s == nil || !period.Valid() {
		return
	}
	s.mu.Lock()
	s.ensure()
	key := liveKey{symbol, period}
	series := s.hist[key]
	switch {
	case len(series) == 0 || bar.Time.After(series[len(series)-1].Time):
		series = append(series, bar)
		s.hist[key] = series
	case bar.Time.Equal(series[len(series)-1].Time):
		series[len(series)-1] = bar
	}
	handler := s.subs[key]
	s.mu.Unlock()
	if handler != nil {
		handler(bar)
	}
}

func (s *StubLiveSource) ensure() {
	if s.hist == nil {
		s.hist = map[liveKey][]Candle{}
	}
	if s.subs == nil {
		s.subs = map[liveKey]BarHandler{}
	}
}

func cloneCandles(bars []Candle) []Candle {
	if len(bars) == 0 {
		return []Candle{}
	}
	out := make([]Candle, len(bars))
	copy(out, bars)
	return out
}
