package kline

import (
	"errors"
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	// LayoutStack is one column of panes per symbol.
	LayoutStack = "stack"
	// LayoutOverlay draws every symbol on one price plot. The first
	// symbol stays candles. The second is a close line. Both are indexed
	// to 100 at the first visible common bar.
	LayoutOverlay = "overlay"
)

const (
	// ToolPan drags the shared viewport.
	ToolPan = "pan"
	// ToolHLine places a horizontal price line.
	ToolHLine = "hline"
	// ToolSegment places a two-point segment.
	ToolSegment = "segment"
	// ToolFib places a two-point Fibonacci retracement.
	ToolFib = "fib"
)

// ErrNilSource is returned when SetSource is called with a nil source.
var ErrNilSource = errors.New("kline: nil data source")

// ErrUnknownIndicator is returned when the name is not in Registry.
var ErrUnknownIndicator = errors.New("kline: unknown indicator")

// ErrUnknownTool is returned when SetTool does not know the name.
var ErrUnknownTool = errors.New("kline: unknown tool")

// ChartOptions configures NewChart. Zero values pick the demo defaults:
// SyntheticSource, symbols ALPHA and BETA, period 1h, stack layout,
// MA and MACD on, magnet off, hollow off, and colors from the window theme.
type ChartOptions struct {
	Source  DataSource
	Symbols []string // e.g. ALPHA, BETA
	Period  Period
	Layout  string // LayoutStack or LayoutOverlay
	Style   Style
	Hollow  bool
	// Indicators names the registry entries that start on, for example
	// "MA", "MACD", "BOLL", "KDJ". Nil keeps MA and MACD on. An empty
	// slice turns every indicator off. Unknown names are ignored.
	Indicators []string
	Magnet     MagnetMode
}

// NewChart loads the initial window and returns a chart the host can
// embed. A source that fails the first GetBars leaves the panes empty;
// SetPeriod reports that error. NewChart does not open a network connection.
func NewChart(opts ChartOptions) *Chart {
	src := opts.Source
	if src == nil {
		src = &SyntheticSource{}
	}
	symbols := append([]string(nil), opts.Symbols...)
	if len(symbols) == 0 {
		symbols = []string{"ALPHA", "BETA"}
	}
	period := opts.Period
	if !period.Valid() {
		period = Period{Span: 1, Unit: UnitHour}
	}
	layout := layoutStack
	if opts.Layout == LayoutOverlay {
		layout = layoutOverlay
	}
	reg := orderedSpecs(Registry())
	explicit := opts.Indicators != nil
	selected := make(map[string]bool, len(opts.Indicators))
	for _, name := range opts.Indicators {
		selected[name] = true
	}
	slots := make([]indicatorSlot, len(reg))
	for i, spec := range reg {
		on := spec.Name == "MA" || spec.Name == "MACD"
		if explicit {
			on = selected[spec.Name]
		}
		slots[i] = indicatorSlot{spec: spec, on: on}
	}
	panes := make([]series, len(symbols))
	for i, name := range symbols {
		panes[i] = series{name: name}
	}
	a := &Chart{
		src:        src,
		period:     period,
		panes:      panes,
		slots:      slots,
		vp:         Viewport{BarWidth: DefaultBarWidth},
		crossIndex: -1,
		crossPane:  -1,
		magnet:     opts.Magnet,
		layout:     layout,
		hollowUp:   opts.Hollow || opts.Style.HollowUp,
		style:      opts.Style,
		styleSet:   styleSpecified(opts.Style),
		limitPct:   opts.Style.LimitPct,
	}
	_ = a.setPeriod(period)
	return a
}

func styleSpecified(s Style) bool {
	s.HollowUp = false
	s.LimitPct = 0
	return s != Style{}
}

// SetStyle replaces the theme colors. HollowUp and LimitPct on s become
// the live hollow switch and the limit threshold. A later SetHollow or
// SetLimitPct updates those two without clearing the rest of the override.
func (a *Chart) SetStyle(s Style) {
	a.style = s
	a.styleSet = true
	a.hollowUp = s.HollowUp
	a.limitPct = s.LimitPct
}

// SetLimitPct sets the close-to-close fraction that paints LimitUp and
// LimitDown. Zero or negative keeps every body on Up and Down. The sample
// walk rarely moves 10% in one bar; a host with a real limit rule sets
// the fraction that rule uses, such as 0.10.
func (a *Chart) SetLimitPct(pct float64) {
	if pct < 0 || math.IsNaN(pct) {
		pct = 0
	}
	a.limitPct = pct
	if a.styleSet {
		a.style.LimitPct = pct
	}
}

// LimitPct is the live limit threshold. Zero means limit colors are off.
func (a *Chart) LimitPct() float64 { return a.limitPct }

// SetPeriod reloads every symbol at p and fits the latest bars when the
// plot width is already known. Drawings are dropped. An invalid period
// returns ErrInvalidPeriod and leaves the current bars in place.
func (a *Chart) SetPeriod(p Period) error { return a.setPeriod(p) }

// Period is the bar size currently loaded.
func (a *Chart) Period() Period { return a.period }

// SetLayout switches between LayoutStack and LayoutOverlay. Any other
// string leaves the layout as it is. A change clears an in-progress drawing.
func (a *Chart) SetLayout(layout string) {
	next := a.layout
	switch layout {
	case LayoutStack:
		next = layoutStack
	case LayoutOverlay:
		next = layoutOverlay
	default:
		return
	}
	if next == a.layout {
		return
	}
	a.layout = next
	a.draftOn = false
	a.dragOn = false
}

// Layout is LayoutStack or LayoutOverlay.
func (a *Chart) Layout() string {
	if a.layout == layoutOverlay {
		return LayoutOverlay
	}
	return LayoutStack
}

// SetSource replaces the bar source and reloads the current period.
// A live subscription on the previous source is dropped. Nil returns
// ErrNilSource. A failed reload restores the previous source.
func (a *Chart) SetSource(src DataSource) error {
	if src == nil {
		return ErrNilSource
	}
	prev := a.src
	a.src = src
	if err := a.setPeriod(a.period); err != nil {
		a.src = prev
		return err
	}
	return nil
}

// SetHollow draws rising candle bodies as outlines when on is true.
func (a *Chart) SetHollow(on bool) { a.hollowUp = on }

// Hollow reports whether rising bodies are outlines.
func (a *Chart) Hollow() bool { return a.hollowUp }

// HollowEnabled is the flag a host ui.Toggle can bind. It is the same
// value SetHollow writes.
func (a *Chart) HollowEnabled() *bool { return &a.hollowUp }

// SetIndicator turns a registry indicator on or off for every symbol.
// Volume is not an indicator and has no name here.
func (a *Chart) SetIndicator(name string, on bool) error {
	for i := range a.slots {
		if a.slots[i].spec.Name == name {
			a.slots[i].on = on
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnknownIndicator, name)
}

// IndicatorOn reports whether name is currently drawn.
func (a *Chart) IndicatorOn(name string) bool {
	for _, slot := range a.slots {
		if slot.spec.Name == name {
			return slot.on
		}
	}
	return false
}

// IndicatorNames is the toolbar order: MA, MACD, BOLL, KDJ, then any
// other registered names.
func (a *Chart) IndicatorNames() []string {
	out := make([]string, len(a.slots))
	for i, slot := range a.slots {
		out[i] = slot.spec.Name
	}
	return out
}

// IndicatorEnabled is the flag a host ui.Toggle can bind for name.
// Unknown names return nil. The pointer stays valid for the life of the chart.
func (a *Chart) IndicatorEnabled(name string) *bool {
	for i := range a.slots {
		if a.slots[i].spec.Name == name {
			return &a.slots[i].on
		}
	}
	return nil
}

// SetMagnet sets how new overlay points snap. It does not move points
// already placed.
func (a *Chart) SetMagnet(mode MagnetMode) { a.magnet = mode }

// Magnet is the current snap mode.
func (a *Chart) Magnet() MagnetMode { return a.magnet }

// SetTool selects pan, hline, segment, or fib. A change clears an
// in-progress drawing. Unknown names return ErrUnknownTool.
func (a *Chart) SetTool(name string) error {
	var tool int
	switch name {
	case ToolPan:
		tool = toolPan
	case ToolHLine:
		tool = toolHLine
	case ToolSegment:
		tool = toolSegment
	case ToolFib:
		tool = toolFib
	default:
		return fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
	if a.tool == tool {
		return nil
	}
	a.tool = tool
	a.draftOn = false
	a.dragOn = false
	return nil
}

// Tool is ToolPan, ToolHLine, ToolSegment, or ToolFib.
func (a *Chart) Tool() string {
	switch a.tool {
	case toolHLine:
		return ToolHLine
	case toolSegment:
		return ToolSegment
	case toolFib:
		return ToolFib
	default:
		return ToolPan
	}
}

// ResetView returns the viewport to the default bar width and fits the
// latest bars on the next frame.
func (a *Chart) ResetView() {
	a.vp.BarWidth = DefaultBarWidth
	a.fitted = false
}

// Bars is the number of bars the shared viewport can address, the
// shortest loaded series.
func (a *Chart) Bars() int { return a.bars() }

// Symbols is the symbol list, in pane order.
func (a *Chart) Symbols() []string {
	out := make([]string, len(a.panes))
	for i, s := range a.panes {
		out[i] = s.name
	}
	return out
}

// Style is the colors View will paint with for theme. A style passed to
// NewChart replaces the theme colors. The hollow switch is the live one.
func (a *Chart) Style(theme *ui.Theme) Style { return a.chartStyle(theme) }
