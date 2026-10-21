package kline

import (
	"errors"
	"fmt"
	"sync"

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

// BarTail is optional. Append adds the bar that is now the last element
// of cs. Revise replaces that last bar; prev is the candle it replaced.
// Both leave the same values as Calc on cs. A chart calls them so a new
// bar does not walk the whole series. Indicators that do not implement
// BarTail are recalculated.
type BarTail interface {
	Append(cs []Candle)
	Revise(cs []Candle, prev Candle)
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

// NamedValues is optional. An indicator implements it to publish its
// numbers by name to Chart.IndicatorValues, which is how a host draws a
// header in its own language and precision instead of parsing ValueAt.
//
// Keys are fixed names, not sentences: MA5, MA10, MACD.DIF, KDJ.K, and so
// on. A key is absent when that value is not ready at i (a warm-up gap),
// which is the same condition that hides it from ValueAt. Chart adds the
// keys of every enabled indicator, built-in or host-registered, to one
// map, so a host reads a value without knowing which indicator owns it.
type NamedValues interface {
	Values(i int) map[string]float64
}

// ErrDuplicateIndicator is returned when Register is called with a
// name that is already in the list.
var ErrDuplicateIndicator = errors.New("kline: duplicate indicator")

// ErrIndicatorKind is returned when Spec.Kind is not the PaneKind of
// the indicator New returns. Layout follows the instance, so a spec
// registered as a sub-pane whose constructor returns a main-pane
// indicator would draw on the wrong plot. Register refuses that.
var ErrIndicatorKind = errors.New("kline: indicator kind does not match its constructor")

// Spec is one entry in the indicator list. New returns a fresh
// indicator for one symbol.
type Spec struct {
	Name string
	Kind string
	New  func() Indicator
}

var (
	registryMu         sync.RWMutex
	registry           []Spec
	reservedIndicators = map[string]bool{
		"MA": true, "MACD": true, "BOLL": true, "KDJ": true,
	}
)

// Register appends an indicator. Name, Kind, and New are required.
// Kind is PaneMain or PaneSub, and New().PaneKind must return that
// same kind. A mismatch returns ErrIndicatorKind and does not
// register. A repeated name returns ErrDuplicateIndicator and leaves
// the list unchanged. Call it from an init function, before NewChart.
// A chart that already exists picks the spec up from SyncIndicators.
// Register, Unregister, and Registry can be called from any goroutine.
func Register(spec Spec) error {
	return register(spec, false)
}

func register(spec Spec, builtin bool) error {
	if spec.Name == "" || spec.New == nil {
		return fmt.Errorf("kline: indicator %q needs a name and constructor", spec.Name)
	}
	if spec.Kind != PaneMain && spec.Kind != PaneSub {
		return fmt.Errorf("kline: indicator %q kind %q", spec.Name, spec.Kind)
	}
	ind := spec.New()
	if ind == nil {
		return fmt.Errorf("kline: indicator %q constructor returned nil", spec.Name)
	}
	if got := ind.PaneKind(); got != spec.Kind {
		return fmt.Errorf("%w: %s registered %s, constructor returned %s", ErrIndicatorKind, spec.Name, spec.Kind, got)
	}
	if !builtin && reservedIndicators[spec.Name] {
		return fmt.Errorf("%w: %s is reserved", ErrDuplicateIndicator, spec.Name)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, have := range registry {
		if have.Name == spec.Name && (builtin || !reservedIndicators[spec.Name]) {
			return fmt.Errorf("%w: %s", ErrDuplicateIndicator, spec.Name)
		}
	}
	registry = append(registry, spec)
	return nil
}

// Unregister removes a host indicator. An unknown or built-in name is a
// no-op. A chart that already copied the spec keeps it until SyncIndicators.
func Unregister(name string) {
	if reservedIndicators[name] {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	next := make([]Spec, 0, len(registry))
	for _, spec := range registry {
		if spec.Name != name {
			next = append(next, spec)
		}
	}
	registry = next
}

func mustRegister(spec Spec) {
	// Package init creates the built-ins that define the reserved set.
	if err := register(spec, true); err != nil {
		panic(err)
	}
}

// Registry returns a copy of the registered indicator list.
func Registry() []Spec {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Spec, len(registry))
	copy(out, registry)
	return out
}
