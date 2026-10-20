package kline

import (
	"errors"
	"fmt"
	"time"
)

const (
	// UnitMinute, UnitHour, and UnitDay are the period units a source accepts.
	UnitMinute = "minute"
	UnitHour   = "hour"
	UnitDay    = "day"

	// KindInit loads the latest window. KindForward loads bars strictly
	// before Anchor. KindBackward is reserved; SyntheticSource rejects it.
	KindInit     = "init"
	KindForward  = "forward"
	KindBackward = "backward"

	// DefaultLimit is the init window when BarRequest.Limit is unset.
	DefaultLimit = 600
)

// ErrInvalidPeriod is returned when Span or Unit cannot form a bar size.
var ErrInvalidPeriod = errors.New("kline: invalid period")

// ErrUnsupportedKind is returned for a request kind the source does not serve.
var ErrUnsupportedKind = errors.New("kline: unsupported bar request kind")

// Period is a bar size. Span is the count of Unit.
type Period struct {
	Span int
	Unit string
}

// Valid reports whether p is a positive span of a known unit.
func (p Period) Valid() bool {
	if p.Span <= 0 {
		return false
	}
	switch p.Unit {
	case UnitMinute, UnitHour, UnitDay:
		return true
	default:
		return false
	}
}

// Duration returns the bar width. Invalid periods return ErrInvalidPeriod.
func (p Period) Duration() (time.Duration, error) {
	if !p.Valid() {
		return 0, ErrInvalidPeriod
	}
	switch p.Unit {
	case UnitMinute:
		return time.Duration(p.Span) * time.Minute, nil
	case UnitHour:
		return time.Duration(p.Span) * time.Hour, nil
	case UnitDay:
		return time.Duration(p.Span) * 24 * time.Hour, nil
	default:
		return 0, ErrInvalidPeriod
	}
}

// String renders a short label such as "1h", "4h", or "1d".
func (p Period) String() string {
	switch p.Unit {
	case UnitMinute:
		return fmt.Sprintf("%dm", p.Span)
	case UnitHour:
		return fmt.Sprintf("%dh", p.Span)
	case UnitDay:
		return fmt.Sprintf("%dd", p.Span)
	default:
		return fmt.Sprintf("%d %s", p.Span, p.Unit)
	}
}

// BarRequest asks a DataSource for one window of bars.
// Anchor is the open time of the earliest loaded bar for forward, and of
// the latest loaded bar for backward. Init ignores a zero Anchor.
type BarRequest struct {
	Symbol string
	Period Period
	Kind   string
	Anchor time.Time
	Limit  int
}

// DataSource loads chart bars. more is true when bars may still exist
// beyond the returned window, in the direction of Kind. The chart copies
// the returned slice before it calls GetBars again, so a source may reuse
// one buffer across calls and symbols.
type DataSource interface {
	GetBars(req BarRequest) (bars []Candle, more bool, err error)
}

// ErrNilBarHandler is returned when SubscribeBar is called without a handler.
var ErrNilBarHandler = errors.New("kline: nil bar handler")

// BarHandler receives one bar update. The bar is either a new open time
// or a revision of the last bar. The caller chooses the goroutine.
// A handler installed by Chart must not call SetPeriod, SetSource, or View.
type BarHandler func(bar Candle)

// LiveSource is an optional extension of DataSource. A feed that can
// push bars implements it. GetBars is still how a chart loads history.
// This package does not open a network connection.
type LiveSource interface {
	DataSource
	SubscribeBar(symbol string, period Period, onBar BarHandler) error
	UnsubscribeBar(symbol string, period Period) error
}
