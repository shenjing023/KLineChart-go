package kline

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// LayoutStack is one column of panes per symbol.
	LayoutStack = "stack"
	// LayoutOverlay compares the first two symbols on one price plot. The
	// first stays candles. The second is a close line. Both are indexed
	// to 100 at the first visible bar where both closes are positive.
	// Further symbols are not drawn and have no header until LayoutStack.
	LayoutOverlay = "overlay"
)

const (
	// ToolPan drags the shared viewport.
	ToolPan = "pan"
	// ToolHLine places a horizontal price line.
	ToolHLine = "hline"
	// ToolSegment places a two-point segment.
	ToolSegment = "segment"
	// ToolFib places a two-point Fibonacci retracement. It is the same
	// string as OverlayFibonacci, the kind stored on the drawing, so
	// Tool() compares equal to Overlay.Kind. SetTool also accepts the
	// older name "fib".
	ToolFib = OverlayFibonacci
)

// ErrNilSource is returned when SetSource is called with a nil source.
var ErrNilSource = errors.New("kline: nil data source")

// ErrUnknownIndicator is returned when the name is not in Registry.
var ErrUnknownIndicator = errors.New("kline: unknown indicator")

// ErrUnknownTool is returned when SetTool does not know the name.
var ErrUnknownTool = errors.New("kline: unknown tool")

// ErrUnknownLayout is returned when SetLayout is not given LayoutStack
// or LayoutOverlay. The layout already in use stays.
var ErrUnknownLayout = errors.New("kline: unknown layout")

// ErrUnknownMagnet is returned when SetMagnet is not given MagnetOff,
// MagnetWeak, or MagnetStrong. The mode already in use stays.
var ErrUnknownMagnet = errors.New("kline: unknown magnet mode")

// ErrDuplicateSymbol is LoadError when ChartOptions.Symbols names a symbol
// twice. Such a chart has no panes, and every load returns this error.
var ErrDuplicateSymbol = errors.New("kline: duplicate symbol")

// ErrMisaligned is returned when the symbols' bars cannot share one bar
// index. Every pane uses the same index: bar i is the same open time on
// each symbol. A load whose windows still disagree after trimming to the
// open times they share is refused. AlignError reports forward and live
// bars that had to be left out after a load.
var ErrMisaligned = errors.New("kline: symbols do not share bar open times")

// ChartOptions configures NewChart. Zero values pick the A-share demo
// defaults: SyntheticSource, symbols 600519 and 000001, period 1h, stack
// layout, MA and MACD on, magnet off, hollow off, Asia/Shanghai labels,
// and red-up/green-down colors from the window theme.
type ChartOptions struct {
	Source DataSource
	// Symbols are the panes, top to bottom, e.g. 600519 and 000001. A
	// name may appear once: a repeated name leaves the chart without panes,
	// and LoadError wraps ErrDuplicateSymbol. Every symbol shares one bar
	// index, so the source must report the same open times for each; see
	// ErrMisaligned.
	Symbols []string
	Period  Period
	Layout  string // LayoutStack or LayoutOverlay
	Style   Style
	Hollow  bool
	// Indicators names the registry entries that start on, for example
	// "MA", "MACD", "BOLL", "KDJ". Nil keeps MA and MACD on. An empty
	// slice turns every indicator off. Unknown names are ignored.
	// SetIndicators reads nil differently: there it turns every
	// indicator off.
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
	// PriceDecimals is the number of digits after the decimal in prices
	// the chart formats: Readout (FormatBar) and the crosshair price tags.
	// Nil keeps two decimals, which is FormatPrice. A negative value also
	// keeps two. Zero prints a whole number. Indicator ValueAt strings
	// still call FormatPrice and stay at two decimals.
	PriceDecimals *int
	// Location formats the times in Readout and the crosshair time tag.
	// Nil uses Asia/Shanghai for the A-share default; pass time.UTC for
	// UTC or another zone explicitly. Candle.Time itself is not rewritten.
	Location *time.Location
	// FormatTime replaces the text of every time the chart prints: the
	// time-axis ticks, the crosshair time tag, and the time in Readout
	// and the headers. It receives the instant already in Location, so a
	// host never converts zones twice. Nil keeps the built-in layouts.
	//
	// Only the text changes. Tick positions, marker placement,
	// aggregation buckets, and the replay cursor all keep working on the
	// real time, so a mask such as "xxxx-xx-02" cannot move a drawing or
	// leak a date through the axis spacing.
	FormatTime func(time.Time) string
	// FormatTick is FormatTime plus the role of the label, so a host can
	// use a different granularity in each place. A blind session needs
	// exactly that: the axis wants the year and month, because month
	// ticks all land on the first of a month and a day mask would print
	// the same string seven times; the crosshair and the readout want
	// the day.
	//
	// It takes precedence over FormatTime when both are set. Like
	// FormatTime it changes only text: tick positions, marker
	// placement, and the replay cursor stay on the real time.
	FormatTick func(t time.Time, kind TickKind) string
	// FormatVolume replaces the text of a volume-axis tick, the same way
	// FormatTime replaces a time: a host that quotes volume in 万手 or
	// 亿手 formats the number and the tick keeps its position. It gets
	// the tick's value, already rounded to a whole number when it is
	// finite. Nil prints the plain number. The crosshair readout keeps
	// FormatVolume's own text, so a host that wants both to match uses
	// this for its own header too.
	FormatVolume func(float64) string
	// MAPeriods overrides the periods of the built-in MA indicator. Nil
	// keeps the default 5 and 20. A host that wants four lines, or a
	// subset of them, sets them here; a period below 1 is dropped, the
	// same rule NewMA uses. The switch is still the "MA" toggle, and
	// SetIndicators("MA") keeps working.
	MAPeriods []int
	// MAColors colors the MA lines in order, one per MAPeriods entry. Nil
	// keeps Style.MA5 and Style.MA20, repeating them for extra lines. A
	// short list leaves the rest on the built-in pattern rather than
	// transparent, so a host may color only the first two.
	MAColors []ui.Color
	// SymbolLabels is the display name of each symbol, parallel to
	// Symbols: the title a screen reader announces and the name the
	// chart's own box carries. An empty entry falls back to the symbol.
	// A host that must not leak a ticker during a blind session passes a
	// neutral name here.
	SymbolLabels []string
	// PaneHeights overrides the sub-pane bands, in DIPs. Zero keeps the
	// built-in share of the plot, which is what the demo uses. A
	// positive Sub applies to every open sub-pane; Volume applies to the
	// built-in volume strip. The price plot takes what is left, and the
	// layout still compresses the bands when the box is too short, so a
	// host never has to compute a height that fits.
	PaneHeights PaneHeights
	// BarColor decides the color of one candle and its volume bar. It is
	// how a host applies a rule the library cannot know, such as a limit
	// price computed from the previous close and rounded to cents. i is
	// the index in the full series, not in the visible window. c and prev
	// are the original bars: an overlay scales prices to an index, and
	// the callback always sees the raw OHLCV. prev is the zero Candle for
	// i == 0.
	//
	// A zero ui.Color falls back to the built-in rule (Style.Up, Down,
	// LimitUp, LimitDown from Style.LimitPct), so a host only answers for
	// the bars it has an opinion about. The volume bar uses the same
	// answer, which keeps a limit-up candle and its volume one color.
	BarColor func(i int, c, prev Candle) ui.Color
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
		symbols = []string{"600519", "000001"}
	}
	display := make([]string, len(symbols))
	for i := range display {
		display[i] = symbols[i]
		if i < len(opts.SymbolLabels) && opts.SymbolLabels[i] != "" {
			display[i] = opts.SymbolLabels[i]
		}
	}
	symbolErr := uniqueSymbols(symbols)
	if symbolErr != nil {
		symbols = nil
	}
	period := opts.Period
	// A week is valid but no source serves it, so a chart cannot open on
	// one. It falls back to the demo default like any other unusable
	// period; SetPeriod reports the reason for a host that asks later.
	if !period.Valid() || !period.Loadable() {
		period = Period{Span: 1, Unit: UnitHour}
	}
	zone := opts.Location
	if zone == nil {
		zone = ashareLocation
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
	slots := make([]*indicatorSlot, len(reg))
	for i, spec := range reg {
		on := spec.Name == "MA" || spec.Name == "MACD"
		if explicit {
			on = selected[spec.Name]
		}
		slots[i] = &indicatorSlot{spec: spec, on: on}
	}
	panes := make([]series, len(symbols))
	for i, name := range symbols {
		panes[i] = series{name: name}
	}
	a := &Chart{
		src:          src,
		period:       period,
		panes:        panes,
		slots:        slots,
		vp:           Viewport{BarWidth: DefaultBarWidth},
		crossIndex:   -1,
		crossPane:    -1,
		magnet:       knownMagnet(opts.Magnet),
		layout:       layout,
		hollowUp:     opts.Hollow || opts.Style.HollowUp,
		style:        opts.Style,
		styleSet:     styleSpecified(opts.Style),
		limitPct:     opts.Style.LimitPct,
		hideHeader:   opts.HideHeader,
		marshal:      opts.Marshal,
		symbolErr:    symbolErr,
		priceDigits:  priceDigitsOf(opts.PriceDecimals),
		digitsSet:    true,
		zone:         zone,
		timeFormat:   opts.FormatTime,
		tickFormat:   opts.FormatTick,
		volFormat:    opts.FormatVolume,
		barColor:     opts.BarColor,
		maPeriods:    append([]int(nil), opts.MAPeriods...),
		maColors:     append([]ui.Color(nil), opts.MAColors...),
		displayNames: display,
		paneHeights:  opts.PaneHeights,
	}
	if a.styleSet {
		a.style = completeStyle(a.style)
		a.style.HollowUp = a.hollowUp
		a.style.LimitPct = a.limitPct
	}
	a.loadErr = a.setPeriod(period)
	return a
}

func uniqueSymbols(symbols []string) error {
	seen := make(map[string]bool, len(symbols))
	for _, name := range symbols {
		if seen[name] {
			return fmt.Errorf("%w: %s", ErrDuplicateSymbol, name)
		}
		seen[name] = true
	}
	return nil
}

func styleSpecified(s Style) bool {
	if s.Clear != 0 {
		return true
	}
	s.HollowUp = false
	s.LimitPct = 0
	s.Clear = 0
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
// loads again, as it does after Close because Close is not final. An
// invalid period returns ErrInvalidPeriod without asking the source,
// and leaves the current bars in place. A GetBars error also leaves
// them in place. When the source implements LiveSource, a SubscribeBar
// error is returned after the new bars are loaded.
// Subscriptions opened for that call are rolled back, and a handler from
// the previous subscription does not apply.
//
// During replay, SetPeriod does not ask the source. A coarser period
// aggregates only the revealed bars, including a partial last bucket. A
// daily bucket opens on the first revealed bar of its local day, in
// ChartOptions.Location, so it names the same instant a plain daily load
// does; hourly and minute buckets stay epoch-aligned.
// A finer period returns ErrReplayPeriod and leaves the cursor in place.
func (a *Chart) SetPeriod(p Period) error {
	if a != nil && a.replay != nil {
		return a.replay.setPeriod(p)
	}
	if !p.Valid() {
		return ErrInvalidPeriod
	}
	if p == a.period && a.loadErr == nil && !a.closed {
		return nil
	}
	return a.setPeriod(p)
}

// LoadError is why the current period has no bars or no live updates: the
// GetBars or ErrMisaligned error of the load NewChart ran, the SubscribeBar
// error of the load that installed the current bars, or ErrDuplicateSymbol.
// It is nil once the current period and source loaded and subscribed. A
// SetPeriod or SetSource whose load fails returns that error and keeps the
// previous bars, so it does not change LoadError.
func (a *Chart) LoadError() error { return a.loadErr }

// AlignError reports bars left off the shared bar index since the last
// load, wrapping ErrMisaligned: a forward batch whose open times do not
// line up with the other symbols' at the left edge, or a held live bar
// dropped because another symbol skipped its open time. The bars on
// screen stay aligned either way. A load clears it.
func (a *Chart) AlignError() error { return a.alignErr }

// Close unsubscribes every live bar and fences handlers already handed
// to the feed. A handler that runs after Close does not change the chart.
// Close does not release the DataSource. A second call returns nil.
// A nil *Chart is accepted, so a host may defer Close before it has
// built one; no other method accepts nil.
func (a *Chart) Close() error {
	if a == nil {
		return nil
	}
	if a.replay != nil {
		a.replay.Close()
	}
	a.closed = true
	return a.dropLive()
}

// Period is the bar size currently loaded.
func (a *Chart) Period() Period { return a.period }

// SetLayout switches between LayoutStack and LayoutOverlay. Any other
// string returns ErrUnknownLayout and leaves the layout as it is. A
// change clears an in-progress drawing. NewChart still treats an empty
// or unknown Layout option as stack; this method is what reports a typo.
func (a *Chart) SetLayout(layout string) error {
	next := a.layout
	switch layout {
	case LayoutStack:
		next = layoutStack
	case LayoutOverlay:
		next = layoutOverlay
	default:
		return fmt.Errorf("%w: %s", ErrUnknownLayout, layout)
	}
	if next == a.layout {
		return nil
	}
	a.layout = next
	a.draftOn = false
	a.dragOn = false
	return nil
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
// its drawings, and its subscription, and leaves an attached replay
// running. A successful load ends the replay, because the new bars were
// not revealed by its cursor. A SubscribeBar error is returned after the
// new source's bars are loaded; that source stays, and subscriptions
// opened for the call are rolled back. The handler must not call
// SetPeriod, SetSource, or View.
func (a *Chart) SetSource(src DataSource) error {
	if src == nil {
		return ErrNilSource
	}
	if a.symbolErr != nil {
		return a.symbolErr
	}
	if !a.period.Valid() {
		return ErrInvalidPeriod
	}
	// Load before touching the replay: a refused source or a failed
	// GetBars must leave playback where it was.
	next, dropErr, err := a.fetchInit(src, a.period)
	if err != nil {
		return err
	}
	if a.replay != nil {
		a.replay.Close()
	}
	a.src = src
	return a.install(a.period, next, dropErr)
}

// SetHollow draws rising candle bodies as outlines when on is true.
// When the chart has its own style, the stored Style.HollowUp is
// updated too, the same way SetLimitPct writes Style.LimitPct.
// chartStyle still paints from the live switch.
func (a *Chart) SetHollow(on bool) {
	a.hollowUp = on
	if a.styleSet {
		a.style.HollowUp = on
	}
}

// Hollow reports whether rising bodies are outlines.
func (a *Chart) Hollow() bool { return a.hollowUp }

// HollowEnabled is the flag a host ui.Toggle can bind. It is the same
// value SetHollow writes.
func (a *Chart) HollowEnabled() *bool { return &a.hollowUp }

// SetMA replaces the built-in MA periods and line colors. Empty periods
// keep the registry default of 5 and 20. Each symbol's MA is rebuilt and
// recalculated on the next paint; the "MA" toggle and its bindable
// pointer are unchanged, so a host does not have to rebind a checkbox.
func (a *Chart) SetMA(periods []int, colors []ui.Color) {
	a.maPeriods = append([]int(nil), periods...)
	a.maColors = append([]ui.Color(nil), colors...)
	if len(periods) == 0 {
		return
	}
	for i := range a.panes {
		s := &a.panes[i]
		a.ensureInds(s)
		for j, slot := range a.slots {
			if slot.spec.Name != "MA" || j >= len(s.inds) {
				continue
			}
			s.inds[j] = a.newIndicator(slot.spec)
			s.fresh[j] = false
		}
		a.recalc(s)
	}
}

// SetSymbolLabels replaces the accessible display names, parallel to
// Symbols. A short list, or an empty entry, falls back to the symbol, so
// a host may label only the pane it needs to hide.
func (a *Chart) SetSymbolLabels(labels []string) {
	display := make([]string, len(a.panes))
	for i := range display {
		display[i] = a.panes[i].name
		if i < len(labels) && labels[i] != "" {
			display[i] = labels[i]
		}
	}
	a.displayNames = display
}

// SetPaneHeights replaces the sub-pane band override. Zero fields keep
// the built-in share of the plot.
func (a *Chart) SetPaneHeights(h PaneHeights) { a.paneHeights = h }

// PaneHeights is the current band override.
func (a *Chart) PaneHeights() PaneHeights { return a.paneHeights }

// SetIndicators turns on exactly the named indicators and turns the
// others off. Nil and an empty slice both turn every indicator off. That
// differs from ChartOptions.Indicators, where nil keeps the default MA
// and MACD. An unknown name returns ErrUnknownIndicator and changes
// nothing. Volume is not an indicator and has no name here.
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

// newIndicator builds one indicator from a spec, applying the host's MA
// override. A chart that never sets MAPeriods gets exactly what the
// registry constructor returns, so the default palette and periods are
// unchanged.
func (a *Chart) newIndicator(spec Spec) Indicator {
	ind := spec.New()
	if spec.Name != "MA" || len(a.maPeriods) == 0 {
		return ind
	}
	if _, ok := ind.(*MA); ok {
		next := NewMA(a.maPeriods...)
		if len(a.maColors) > 0 {
			// Lines past the host's list keep the theme pair: a short
			// list must not leave a line invisible.
			def := DarkDefault()
			colors := make([]ui.Color, len(next.Periods))
			copy(colors, a.maColors)
			for i := len(a.maColors); i < len(colors); i++ {
				if i%2 == 0 {
					colors[i] = def.MA5
				} else {
					colors[i] = def.MA20
				}
			}
			next.SetColors(colors)
		}
		return next
	}
	return ind
}

// SyncIndicators reloads the registry. Specs registered after NewChart
// show up here, off, until SetIndicator turns them on. Existing entries
// are kept in place: their toggles, the pointers IndicatorEnabled
// returned, and their calculated values all stay, so nothing is
// recalculated.
func (a *Chart) SyncIndicators() {
	reg := orderedSpecs(Registry())
	have := make(map[string]int, len(a.slots))
	for i, slot := range a.slots {
		have[slot.spec.Name] = i
	}
	slots := make([]*indicatorSlot, len(reg))
	from := make([]int, len(reg))
	for i, spec := range reg {
		j, ok := have[spec.Name]
		if !ok {
			slots[i], from[i] = &indicatorSlot{spec: spec}, -1
			continue
		}
		slots[i], from[i] = a.slots[j], j
	}
	for p := range a.panes {
		s := &a.panes[p]
		a.ensureInds(s)
		inds := make([]Indicator, len(slots))
		fresh := make([]bool, len(slots))
		for i, j := range from {
			if j >= 0 {
				inds[i], fresh[i] = s.inds[j], s.fresh[j]
				continue
			}
			// A host MA override applies here too, so an indicator
			// registered after NewChart starts with the same periods as
			// the ones already on the chart.
			inds[i] = a.newIndicator(slots[i].spec)
		}
		s.inds, s.fresh = inds, fresh
	}
	a.slots = slots
}

// Crosshair is the bar under the pointer. ok is false when the pointer
// is outside every plot, including over the price axis. symbol is the
// pane under the pointer; in overlay mode that is the primary symbol.
func (a *Chart) Crosshair() (symbol string, index int, ok bool) {
	if a.crossIndex < 0 {
		return "", -1, false
	}
	if a.crossPane >= 0 && a.crossPane < len(a.panes) {
		symbol = a.panes[a.crossPane].name
	}
	return symbol, a.crossIndex, true
}

// CrosshairTime is the open time of the bar under the pointer, which is
// what two charts of different series share: a host maps a time, not a
// bar index, because a suspended stock has no bar on a day the market
// panel still has one.
func (a *Chart) CrosshairTime() (t time.Time, ok bool) {
	if a.crossIndex < 0 {
		return time.Time{}, false
	}
	c, ok := a.barAt(a.crossIndex)
	if !ok {
		return time.Time{}, false
	}
	return c.Time, true
}

// SetCrosshairTime places the crosshair on the bar whose open interval
// contains t and reports whether this chart has such a bar. A time
// outside the series clears the crosshair and returns false, which is
// what a host wants when the other chart is showing a day this one does
// not trade.
//
// The placed crosshair keeps its bar until the pointer moves on this
// chart or SetCrosshairTime(nil time) is called, so a linked chart can
// render it on every frame without the host re-sending it. It does not
// run OnCrosshairChange: a host that forwards a time must not receive
// its own echo.
func (a *Chart) SetCrosshairTime(t time.Time) bool {
	idx, ok := a.indexAtTime(t)
	if !ok {
		a.clearCrosshairAt()
		return false
	}
	a.crossAt = t
	a.crossAtSet = true
	a.crossFixed = true
	if a.crossIndex != idx {
		a.crossIndex = idx
	}
	if a.crossPane < 0 {
		a.crossPane = 0
	}
	return true
}

// ClearCrosshairTime removes a crosshair placed by SetCrosshairTime.
// A pointer crosshair is unaffected: this clears only what the host set.
func (a *Chart) ClearCrosshairTime() {
	a.clearCrosshairAt()
}

func (a *Chart) clearCrosshairAt() {
	a.crossAtSet = false
	a.crossFixed = false
	a.crossAt = time.Time{}
	if a.crossPane < 0 || a.crossIndex < 0 {
		return
	}
	// A host-placed crosshair is the only one this method may clear.
	// Leave the pointer's own position alone by dropping the index;
	// the next frame rebuilds it from the pointer if the pointer is
	// still over a plot.
	a.crossIndex = -1
	a.crossPane = -1
}

// indexAtTime is the bar index whose open interval contains t. The last
// bar extends one period past its open, the same rule a marker lookup
// uses, so a time that names the last bar's interval lands on it rather
// than falling off the end. A time after that interval is a miss.
func (a *Chart) indexAtTime(t time.Time) (int, bool) {
	if t.IsZero() || len(a.panes) == 0 {
		return 0, false
	}
	cs := a.panes[0].candles
	if len(cs) == 0 {
		return 0, false
	}
	step, _ := a.period.Duration()
	idx := barContaining(cs, t, step)
	return idx, idx >= 0
}

// barAt is the primary series bar at index, clamped away from a
// past-the-end index a pointer may hold after a period change.
func (a *Chart) barAt(index int) (Candle, bool) {
	cs := a.panes[0].candles
	if index < 0 || index >= len(cs) {
		return Candle{}, false
	}
	return cs[index], true
}

// OnCrosshairChange sets the callback for a changed hovered bar. It runs
// inside View, on the frame the change is detected, with the symbol and
// index of the bar under the pointer and ok false when the pointer has
// left every plot. A host that owns two charts calls the other chart's
// SetCrosshairTime here; because the callback runs inside View, a host
// that paints the second chart after the first in the same frame gets a
// linked crosshair without a frame of lag. The callback must not call
// SetPeriod, SetSource, or View on the chart that invoked it. nil
// clears it.
func (a *Chart) OnCrosshairChange(fn func(symbol string, index int, ok bool)) {
	a.crossHook = fn
}

// VisibleTime is the open time of the first and last visible bar. ok is
// false when the plot has no bars on screen yet.
func (a *Chart) VisibleTime() (from, to time.Time, ok bool) {
	first, last := a.Visible()
	if last < first || len(a.panes) == 0 {
		return time.Time{}, time.Time{}, false
	}
	cs := a.panes[0].candles
	if first < 0 || last >= len(cs) {
		return time.Time{}, time.Time{}, false
	}
	return cs[first].Time, cs[last].Time, true
}

// SetVisibleTime pans this chart so the bars between from and to fill
// the plot, matching a second chart's window. It changes the bar width
// only when the span cannot be shown at the current width, so a linked
// pan keeps both charts at the same scale instead of fighting over it.
// A chart with no bars, or a range outside its series, leaves the
// viewport alone and returns false.
func (a *Chart) SetVisibleTime(from, to time.Time) bool {
	if a.plotW <= 1 {
		// Layout has not run, so there is no width to fit into. The
		// host's next frame paints with the default window; asking it to
		// retry would need a signal this chart does not have.
		return false
	}
	first, okFirst := a.indexAtTime(from)
	last, okLast := a.indexAtTime(to)
	if !okFirst || !okLast || last < first {
		return false
	}
	span := last - first + 1
	width := a.plotW / float64(span)
	if width < MinBarWidth {
		width = MinBarWidth
	}
	if width > MaxBarWidth {
		width = MaxBarWidth
	}
	a.vp.BarWidth = width
	// from is placed 15% in from the left so the range does not sit
	// flush against the edge, which reads as "the data ends here".
	a.vp.Offset = float64(first) - (a.plotW/width)*0.15
	a.vp.Clamp(a.bars(), a.plotW)
	a.fitted = true
	return true
}

// ScrollToTime moves the viewport so t is at the right edge, keeping the
// current bar width. A host uses it to park a settled session on its end
// date instead of jumping to the newest bar.
func (a *Chart) ScrollToTime(t time.Time) bool {
	idx, ok := a.indexAtTime(t)
	if !ok || a.plotW <= 1 {
		return false
	}
	if a.vp.BarWidth <= 0 {
		a.vp.BarWidth = DefaultBarWidth
	}
	// The same geometry as FitRight with the target bar standing in for
	// the last one: its right edge lands OverscrollBars in from the
	// plot's right edge, so the bars after t stay off screen until the
	// user drags. Clamp then decides whether the target can reach that
	// far; a target too early in a short series shows as much as fits.
	margin := float64(OverscrollBars) * a.vp.BarWidth
	a.vp.Offset = float64(idx+1) - (a.plotW-margin)/a.vp.BarWidth
	a.vp.Clamp(a.bars(), a.plotW)
	a.fitted = true
	return true
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

// IndicatorValues is the numeric readout for symbol at index: one map
// holding every key of every enabled indicator that has a value there.
// A host uses it to draw a header in its own language, precision, and
// units instead of parsing Readout's English text.
//
// Built-in keys are MA<period> (MA5, MA10, ...), MACD.DIF, MACD.DEA,
// MACD.HIST, BOLL's MID, UP, LOW, and KDJ.K, KDJ.D, KDJ.J. A
// host-registered indicator contributes the keys its Values method
// returns; see NamedValues. A key is absent while its value is not ready
// (a warm-up gap) or when that indicator is off, so a host checks for
// presence rather than NaN. An unknown symbol or an out-of-range index
// returns nil.
//
// The map is freshly allocated for the call; a host that reads it every
// frame may keep one and reuse it with ReadInto.
func (a *Chart) IndicatorValues(symbol string, index int) map[string]float64 {
	out := make(map[string]float64)
	a.ReadIndicatorValues(symbol, index, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// ReadIndicatorValues fills out with the same values IndicatorValues
// returns, so a host that draws a header every frame can reuse one map.
// out is cleared first, which means a key that is no longer ready is
// gone rather than stale. out may be nil, in which case nothing is
// written.
//
// The map still costs what each indicator's Values allocates: a host on
// a tight frame budget should call IndicatorValues only when the hovered
// bar changes, not on every frame.
func (a *Chart) ReadIndicatorValues(symbol string, index int, out map[string]float64) {
	if out == nil {
		return
	}
	clear(out)
	for i := range a.panes {
		s := &a.panes[i]
		if s.name != symbol {
			continue
		}
		if index < 0 || index >= len(s.candles) {
			return
		}
		for _, one := range a.active(s) {
			if nv, ok := one.(NamedValues); ok {
				for k, v := range nv.Values(index) {
					out[k] = v
				}
			}
		}
		return
	}
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

// SetIndicator turns a registry indicator on or off for every symbol. An
// indicator is only calculated while it is on; one turned on after the
// bars changed is calculated when it is next drawn or read. Volume is
// not an indicator and has no name here.
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
// Unknown names return nil. The pointer stays valid for the life of the
// chart, including across SyncIndicators, and writing it is the same as
// SetIndicator.
func (a *Chart) IndicatorEnabled(name string) *bool {
	for i := range a.slots {
		if a.slots[i].spec.Name == name {
			return &a.slots[i].on
		}
	}
	return nil
}

// SetMagnet sets how new overlay points snap. It does not move points
// already placed. A mode other than off, weak, or strong returns
// ErrUnknownMagnet and leaves the current mode in place.
func (a *Chart) SetMagnet(mode MagnetMode) error {
	switch mode {
	case MagnetOff, MagnetWeak, MagnetStrong:
		a.magnet = mode
		return nil
	default:
		return fmt.Errorf("%w: %d", ErrUnknownMagnet, mode)
	}
}

// Magnet is the current snap mode.
func (a *Chart) Magnet() MagnetMode { return a.magnet }

// SetOverlays replaces the drawings on one symbol's pane. It is how a
// host adds a drawing programmatically instead of through a click: a
// marker overlay on a forming bar, a level computed from the data, or a
// restored drawing from a saved session. Points are copied. An empty or
// nil list clears the pane. An unknown symbol returns ErrUnknownSymbol
// and changes nothing.
//
// IDs may be reused; a host that wants to replace one drawing repeatedly
// passes the same ID and the whole list each time. An overlay whose
// Points a registered kind cannot place still paints nothing rather than
// failing the call.
func (a *Chart) SetOverlays(symbol string, items []Overlay) error {
	for i := range a.panes {
		if a.panes[i].name != symbol {
			continue
		}
		layer := &a.panes[i].overlays
		*layer = Layer{}
		for _, item := range items {
			layer.seq++
			o := Overlay{
				ID:       item.ID,
				Kind:     item.Kind,
				Points:   append([]Point(nil), item.Points...),
				Selected: false,
				Data:     item.Data,
			}
			if o.ID == "" {
				o.ID = fmt.Sprintf("ov-%d", layer.seq)
			}
			layer.items = append(layer.items, o)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
}

// Overlays returns a copy of one symbol's drawings, the same list a host
// passed to SetOverlays plus anything the user placed with a tool. An
// unknown symbol returns ErrUnknownSymbol.
func (a *Chart) Overlays(symbol string) ([]Overlay, error) {
	for i := range a.panes {
		if a.panes[i].name == symbol {
			return a.panes[i].overlays.Items(), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
}

// SetTool selects pan, hline, segment, fibonacci, or a kind passed to
// RegisterOverlay. "fib" is the older name of the Fibonacci tool and
// selects the same tool. A change clears an in-progress drawing.
// Unknown names return ErrUnknownTool and leave the current tool in place.
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
	case ToolFib, "fib":
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

// Bars is the number of bars every symbol has on screen. Bar i is the
// same open time on each symbol. Live bars still waiting for another
// symbol to reach their open time are not counted.
func (a *Chart) Bars() int { return a.bars() }

// LastBarForming reports whether the newest bar is a bucket that can
// still change. It is true only for a coarser period during a replay:
// the revealed prefix has not reached the end of that day or week, so
// the next reveal revises this bar instead of appending one. A host
// draws its "in progress" marker from it, for example a dashed frame on
// the last weekly candle.
//
// A plain load, a live feed, and AppendBar always produce a finished bar,
// so this is false there. A replay on the base period is false too: each
// reveal is a whole bar.
func (a *Chart) LastBarForming() bool {
	if a == nil || a.replay == nil {
		return false
	}
	return a.replay.LastBarForming()
}

// LastBarForming is the replay's answer for the attached chart.
func (r *Replay) LastBarForming() bool {
	if r == nil || r.chart == nil || r.cursor <= 0 || len(r.base) == 0 {
		return false
	}
	if !r.aggregated() {
		return false
	}
	idx := r.cursor - 1
	if idx >= len(r.base[0]) {
		return false
	}
	return !r.bucketEndsAt(idx)
}

// bucketEndsAt reports whether the base bar at idx is the last one in
// its aggregate bucket. It answers from the revealed prefix alone: the
// bar after idx may be hidden, and reading it would let the future move
// what the chart paints.
//
// A bucket is finished when no later bar could still join it. For an
// epoch-aligned period that is one step after the bar. For a week it is
// the last local weekday: a Monday-to-Thursday bar can still be joined
// by a later weekday in the same Monday week, and a Friday bar cannot.
// A week whose last trading day is a holiday still reports forming
// until it is revealed, which is the honest answer without a trading
// calendar: the chart cannot prove the bucket is closed.
func (r *Replay) bucketEndsAt(idx int) bool {
	if idx < 0 || idx >= len(r.base[0]) {
		return true
	}
	p := r.chart.period
	if p.Unit == UnitWeek {
		local := r.base[0][idx].Time.In(bucketLocation(r.chart.bucketZone()))
		return local.Weekday() == time.Friday ||
			local.Weekday() == time.Saturday ||
			local.Weekday() == time.Sunday
	}
	d, err := r.basePeriod.Duration()
	if err != nil || d <= 0 {
		return true
	}
	loc := r.chart.bucketZone()
	next := r.base[0][idx].Time.Add(d)
	return !bucketFloor(next, p, loc).Equal(bucketFloor(r.base[0][idx].Time, p, loc))
}

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
