package kline

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

const (
	// PaneMain is an overlay on the price plot.
	PaneMain = "main"
	// PaneSub is a pane under the price plot, with its own Y domain.
	PaneSub = "sub"
)

// Indicator is one calculated series. Calc fills the values the other
// methods read. Each symbol owns its own indicator, because the values
// belong to that symbol's bars.
//
// DrawMain paints an overlay on the price plot. DrawSub paints a
// sub-pane. An indicator implements the one that matches PaneKind and
// leaves the other empty.
type Indicator interface {
	Name() string
	PaneKind() string
	Calc(cs []Candle)
	DrawMain(p *ui.Painter, priceRect ui.Rect, vp Viewport, y chart.Linear)
	DrawSub(p *ui.Painter, rect ui.Rect, vp Viewport)
	// ValueAt is the readout for bar i, or "" when i is outside the
	// series or the value is not ready.
	ValueAt(i int) string
}

// LinePlot is optional. A main-pane indicator implements it so the
// price scale includes Series, and so both layouts can stroke those
// lines. NaN entries are gaps. LineColors aligns with Series; a zero
// color skips that line.
//
// When Series is non-nil, Paint strokes the lines and does not call
// DrawMain. Leave DrawMain empty. Indicators that paint fills or
// markers instead of lines leave Series nil and implement DrawMain.
// Overlay mode does not call DrawMain; a main indicator with no
// Series is not drawn there.
type LinePlot interface {
	Series() [][]float64
	LineColors() []ui.Color
}

// SubPlot is optional. A sub-pane indicator implements it so the Y
// axis matches the visible window. Without it the pane still opens
// and DrawSub is called, on a 0..1 scale.
type SubPlot interface {
	Scale(first, last int) chart.Linear
}

// ErrDuplicateIndicator is returned when Register is called with a
// name that is already in the list.
var ErrDuplicateIndicator = errors.New("kline: duplicate indicator")

// Spec is one entry in the indicator list. New returns a fresh
// indicator for one symbol.
type Spec struct {
	Name string
	Kind string
	New  func() Indicator
}

var registry []Spec

// Register appends an indicator. Name, Kind, and New are required.
// Kind is PaneMain or PaneSub. A repeated name returns
// ErrDuplicateIndicator and leaves the list unchanged. Call it from
// an init function, before NewChart. A chart that already exists
// picks the spec up from SyncIndicators.
func Register(spec Spec) error {
	if spec.Name == "" || spec.New == nil {
		return fmt.Errorf("kline: indicator %q needs a name and constructor", spec.Name)
	}
	if spec.Kind != PaneMain && spec.Kind != PaneSub {
		return fmt.Errorf("kline: indicator %q kind %q", spec.Name, spec.Kind)
	}
	for _, have := range registry {
		if have.Name == spec.Name {
			return fmt.Errorf("%w: %s", ErrDuplicateIndicator, spec.Name)
		}
	}
	registry = append(registry, spec)
	return nil
}

func mustRegister(spec Spec) {
	if err := Register(spec); err != nil {
		panic(err)
	}
}

// Registry returns a copy of the registered indicator list.
func Registry() []Spec {
	out := make([]Spec, len(registry))
	copy(out, registry)
	return out
}
