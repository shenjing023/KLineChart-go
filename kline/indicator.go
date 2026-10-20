package kline

import (
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

// Spec is one entry in the indicator list. New returns a fresh
// indicator for one symbol.
type Spec struct {
	Name string
	Kind string
	New  func() Indicator
}

var registry []Spec

// Register appends an indicator to the list the app mounts. init
// functions in this package call it.
func Register(spec Spec) {
	registry = append(registry, spec)
}

// Registry returns a copy of the registered indicator list.
func Registry() []Spec {
	out := make([]Spec, len(registry))
	copy(out, registry)
	return out
}
