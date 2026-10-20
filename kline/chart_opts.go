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
	// HideHeader skips the per-symbol OHLCV line View normally paints.
	// Readout still returns that text for a host that draws its own bar.
	HideHeader bool
	// Marshal hops each live bar onto the UI thread and should request a
	// frame. Pass (*mygo.Window).Update: it runs the func on the main
	// thread and redraws. mygo.RunOnMain hops and waits, and does not
	// redraw; the host must Invalidate after that. Nil applies the bar
	// synchronously on the caller and does not request a frame, which is
	// what tests use. The func passed to Marshal must not call SetPeriod,
	// SetSource, or View.
	Marshal func(func())
}

// NewChart loads the initial window and returns a chart the host can
// embed. A source that fails the first GetBars leaves the panes empty,
// and a SubscribeBar error leaves the bars without live updates. Either
// one is LoadError; SetPeriod with the same period tries the load again.
// NewChart does not open a network connection.
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
		hideHeader: opts.HideHeader,
		marshal:    opts.Marshal,
	}
	if a.styleSet {
		a.style = completeStyle(a.style)
	}
	a.loadErr = a.setPeriod(period)
	return a
}

func styleSpecified(s Style) bool {
	s.HollowUp = false
	s.LimitPct = 0
	return s != Style{}
}

// SetStyle replaces the theme colors. A style that does not set Plot,
// Grid, or Text is filled from DarkDefault, so setting only Up does not
// clear the grid. HollowUp and LimitPct on s become the live hollow
// switch and the limit threshold. A later SetHollow or SetLimitPct
// updates those two without clearing the rest of the override.
func (a *Chart) SetStyle(s Style) {
	s = completeStyle(s)
	a.style = s
	a.styleSet = true
	a.hollowUp = s.HollowUp
	a.limitPct = s.LimitPct
}

// EditStyle starts from the theme palette, keeps the live hollow switch
// and limit percent, runs edit, and installs the result. Changing one
// color does not clear the rest of the palette. A nil theme is dark.
func (a *Chart) EditStyle(theme *ui.Theme, edit func(*Style)) {
	s := StyleFrom(theme)
	s.HollowUp = a.hollowUp
	s.LimitPct = a.limitPct
	if edit != nil {
		edit(&s)
	}
	a.SetStyle(s)
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
// plot width is already known. Drawings, a drawing in progress, and the
// crosshair are dropped. When p is the period already loaded and
// LoadError is nil, SetPeriod does nothing and returns nil: drawings and
// the live subscription stay. While LoadError is set, the same period
// loads again. An invalid period returns ErrInvalidPeriod without asking
// the source, and leaves the current bars in place. A GetBars error also
// leaves them in place. When the source implements LiveSource, a
// SubscribeBar error is returned after the new bars are loaded.
// Subscriptions opened for that call are rolled back, and a handler from
// the previous subscription does not apply.
func (a *Chart) SetPeriod(p Period) error {
	if !p.Valid() {
		return ErrInvalidPeriod
	}
	if p == a.period && a.loadErr == nil {
		return nil
	}
	return a.setPeriod(p)
}

// LoadError is why the current period has no bars or no live updates: the
// GetBars error of the load NewChart ran, or the SubscribeBar error of the
// load that installed the current bars. It is nil once the current period
// and source loaded and subscribed. A SetPeriod or SetSource whose GetBars
// fails returns that error and keeps the previous bars, so it does not
// change LoadError.
func (a *Chart) LoadError() error { return a.loadErr }

// Close unsubscribes every live bar and fences handlers already handed
// to the feed. A handler that runs after Close does not change the chart.
// Close does not release the DataSource. A second call returns nil.
func (a *Chart) Close() error {
	if a == nil {
		return nil
	}
	return a.dropLive()
}

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

// SetSource replaces the bar source and reloads the current period. Like
// a period change, a successful reload drops drawings, a drawing in
// progress, and the crosshair. It reloads even when src is the source
// already in use, which is how a host refreshes a plain DataSource. A
// live subscription on the previous source is dropped. Nil returns
// ErrNilSource. A failed GetBars keeps the previous source, its bars,
// its drawings, and its subscription. A SubscribeBar error is returned
// after the new source's bars are loaded; that source stays, and
// subscriptions opened for the call are rolled back. The handler must
// not call SetPeriod, SetSource, or View.
func (a *Chart) SetSource(src DataSource) error {
	if src == nil {
		return ErrNilSource
	}
	if !a.period.Valid() {
		return ErrInvalidPeriod
	}
	next, err := a.fetchInit(src, a.period)
	if err != nil {
		return err
	}
	a.src = src
	return a.install(a.period, next)
}

// SetHollow draws rising candle bodies as outlines when on is true.
func (a *Chart) SetHollow(on bool) { a.hollowUp = on }

// Hollow reports whether rising bodies are outlines.
func (a *Chart) Hollow() bool { return a.hollowUp }

// HollowEnabled is the flag a host ui.Toggle can bind. It is the same
// value SetHollow writes.
func (a *Chart) HollowEnabled() *bool { return &a.hollowUp }

// SetIndicators turns on exactly the named indicators and turns the
// others off. Nil and an empty slice turn every indicator off. An
// unknown name returns ErrUnknownIndicator and changes nothing.
// Volume is not an indicator and has no name here.
func (a *Chart) SetIndicators(names []string) error {
	want := make(map[string]bool, len(names))
	for _, name := range names {
		if a.IndicatorEnabled(name) == nil {
			return fmt.Errorf("%w: %s", ErrUnknownIndicator, name)
		}
		want[name] = true
	}
	for i := range a.slots {
		a.slots[i].on = want[a.slots[i].spec.Name]
	}
	return nil
}

// SyncIndicators reloads the registry. Specs registered after NewChart
// show up here, off, until SetIndicator turns them on. Existing toggles
// are kept. Each symbol recalculates.
func (a *Chart) SyncIndicators() {
	reg := orderedSpecs(Registry())
	on := make(map[string]bool, len(a.slots))
	for _, slot := range a.slots {
		on[slot.spec.Name] = slot.on
	}
	slots := make([]indicatorSlot, len(reg))
	for i, spec := range reg {
		slots[i] = indicatorSlot{spec: spec, on: on[spec.Name]}
	}
	a.slots = slots
	for i, s := range a.panes {
		a.panes[i] = a.mount(s, s.candles, s.more)
	}
}

// Crosshair is the bar under the pointer. ok is false when the pointer
// is outside every plot. symbol is the pane under the pointer; in
// overlay mode that is the primary symbol.
func (a *Chart) Crosshair() (symbol string, index int, ok bool) {
	if a.crossIndex < 0 {
		return "", -1, false
	}
	if a.crossPane >= 0 && a.crossPane < len(a.panes) {
		symbol = a.panes[a.crossPane].name
	}
	return symbol, a.crossIndex, true
}

// Bar returns the candle at index for symbol.
func (a *Chart) Bar(symbol string, index int) (Candle, bool) {
	for _, s := range a.panes {
		if s.name != symbol || index < 0 || index >= len(s.candles) {
			continue
		}
		return s.candles[index], true
	}
	return Candle{}, false
}

// Visible is the inclusive bar range currently on screen. last < first
// means the plot has not been laid out yet.
func (a *Chart) Visible() (first, last int) {
	return a.vp.Visible(a.bars(), a.plotW)
}

// Readout is the header line for symbol, including the hovered bar and
// the indicators that are on. It is empty for an unknown symbol.
func (a *Chart) Readout(symbol string) string {
	for i, s := range a.panes {
		if s.name == symbol {
			return a.readout(i)
		}
	}
	return ""
}

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

// SetTool selects pan, hline, segment, fib, or a kind passed to
// RegisterOverlay. A change clears an in-progress drawing. Unknown
// names return ErrUnknownTool and leave the current tool in place.
func (a *Chart) SetTool(name string) error {
	var tool int
	custom := ""
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
		if _, ok := overlayByKind(name); !ok {
			return fmt.Errorf("%w: %s", ErrUnknownTool, name)
		}
		tool = toolCustom
		custom = name
	}
	if a.tool == tool && a.customKind == custom {
		return nil
	}
	a.tool = tool
	a.customKind = custom
	a.draftOn = false
	a.dragOn = false
	a.dragNewHLine = false
	return nil
}

// Tool is ToolPan, ToolHLine, ToolSegment, ToolFib, or the custom
// kind selected with SetTool.
func (a *Chart) Tool() string {
	switch a.tool {
	case toolHLine:
		return ToolHLine
	case toolSegment:
		return ToolSegment
	case toolFib:
		return ToolFib
	case toolCustom:
		if a.customKind != "" {
			return a.customKind
		}
		return ToolPan
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
