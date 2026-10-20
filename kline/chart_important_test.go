package kline

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// histSource serves fixed histories. Init returns the last initN bars of
// each symbol, and forward up to Limit bars strictly before Anchor.
type histSource struct {
	hist  map[string][]Candle
	initN int
}

func (h *histSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	all := h.hist[req.Symbol]
	switch req.Kind {
	case KindInit:
		if len(all) <= h.initN {
			return cloneCandles(all), false, nil
		}
		return cloneCandles(all[len(all)-h.initN:]), true, nil
	case KindForward:
		n := 0
		for n < len(all) && all[n].Time.Before(req.Anchor) {
			n++
		}
		lo := max(0, n-req.Limit)
		return cloneCandles(all[lo:n]), lo > 0, nil
	default:
		return nil, false, ErrUnsupportedKind
	}
}

func assertAligned(t *testing.T, a *Chart) {
	t.Helper()
	n := a.Bars()
	for _, s := range a.panes {
		if len(s.candles) != n {
			t.Fatalf("%s has %d bars, chart has %d", s.name, len(s.candles), n)
		}
		for i := range s.candles {
			if !s.candles[i].Time.Equal(a.panes[0].candles[i].Time) {
				t.Fatalf("bar %d: %s at %s, %s at %s", i, s.name, s.candles[i].Time, a.panes[0].name, a.panes[0].candles[i].Time)
			}
		}
	}
}

// countSource counts GetBars on top of a StubLiveSource that keeps every
// handler it was given.
type countSource struct {
	*keepHandlers
	gets int
}

func (c *countSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	c.gets++
	return c.keepHandlers.GetBars(req)
}

func newCountSource(t *testing.T, period Period, n int, symbols ...string) *countSource {
	t.Helper()
	src := &countSource{keepHandlers: &keepHandlers{StubLiveSource: &StubLiveSource{}}}
	for _, name := range symbols {
		if err := src.SetBars(name, period, Generate(n, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func sameCandles(a, b []Candle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Time.Equal(b[i].Time) || a[i].Open != b[i].Open || a[i].High != b[i].High ||
			a[i].Low != b[i].Low || a[i].Close != b[i].Close || a[i].Volume != b[i].Volume {
			return false
		}
	}
	return true
}

func TestSetPeriodSameKeepsDrawingsAndDraft(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	src := newCountSource(t, hour, 120, "ALPHA", "BETA")
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA"}, Period: hour, Indicators: []string{}})
	tt := ui.NewTester(a.View, 1100, 1000)
	price := a.painted[0].Price
	if err := a.SetTool(ToolHLine); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(price.X+120, price.Y+price.H*0.4)
	if err := a.SetTool(ToolSegment); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(price.X+200, price.Y+price.H*0.3)
	if len(a.panes[0].overlays.Items()) != 1 || !a.draftOn {
		t.Fatalf("setup: drawings %d draft %v", len(a.panes[0].overlays.Items()), a.draftOn)
	}
	gets, subs := src.gets, len(src.handlers)

	if err := a.SetPeriod(hour); err != nil {
		t.Fatal(err)
	}
	if src.gets != gets || len(src.handlers) != subs {
		t.Fatalf("same period reloaded: gets %d -> %d, subscribes %d -> %d", gets, src.gets, subs, len(src.handlers))
	}
	if len(a.panes[0].overlays.Items()) != 1 || !a.draftOn {
		t.Fatalf("same period dropped drawings %d or the draft %v", len(a.panes[0].overlays.Items()), a.draftOn)
	}
	tt.ClickAt(price.X+320, price.Y+price.H*0.6)
	items := a.panes[0].overlays.Items()
	if len(items) != 2 || items[1].Kind != OverlaySegment {
		t.Fatalf("draft did not finish after the no-op: %+v", items)
	}

	bars := a.Bars()
	pushBoth(src.StubLiveSource, a, 1)
	if a.Bars() != bars+1 {
		t.Fatalf("subscription lost: bars %d, want %d", a.Bars(), bars+1)
	}
}

func TestSetPeriodInvalidLeavesBars(t *testing.T) {
	gets := 0
	// This source accepts any period, so only the chart can refuse one.
	src := sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		gets++
		return Generate(50, symbolSeed(req.Symbol)), false, nil
	})
	a := NewChart(ChartOptions{Source: src, Indicators: []string{}})
	if a.LoadError() != nil || a.Bars() != 50 {
		t.Fatalf("setup: err %v bars %d", a.LoadError(), a.Bars())
	}
	period, before := a.Period(), gets
	first := a.panes[0].candles[0].Time
	for _, p := range []Period{{}, {Span: 0, Unit: UnitHour}, {Span: -2, Unit: UnitDay}, {Span: 1, Unit: "week"}} {
		if err := a.SetPeriod(p); !errors.Is(err, ErrInvalidPeriod) {
			t.Fatalf("SetPeriod(%+v) = %v, want ErrInvalidPeriod", p, err)
		}
	}
	if gets != before {
		t.Fatalf("an invalid period reached the source: %d calls", gets-before)
	}
	if a.Period() != period || a.Bars() != 50 || !a.panes[0].candles[0].Time.Equal(first) || a.LoadError() != nil {
		t.Fatalf("invalid period changed the chart: period %s bars %d err %v", a.Period(), a.Bars(), a.LoadError())
	}
}

func TestReloadClearsGesture(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	if err := a.SetTool(ToolSegment); err != nil {
		t.Fatal(err)
	}
	price := a.painted[0].Price
	tt.ClickAt(price.X+120, price.Y+price.H*0.3)
	if !a.draftOn {
		t.Fatal("setup: no segment draft")
	}
	a.dragOn, a.dragNewHLine, a.pressOn = true, true, true
	if err := a.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	if a.draftOn || a.dragOn || a.dragNewHLine || a.pressOn {
		t.Fatalf("period change kept draft %v drag %v new hline %v press %v", a.draftOn, a.dragOn, a.dragNewHLine, a.pressOn)
	}
	// The next click starts a new draft. It does not finish a segment whose
	// first point is an hourly open time the 4h bars do not have.
	tt.Frame()
	tt.ClickAt(price.X+300, price.Y+price.H*0.6)
	if len(a.panes[0].overlays.Items()) != 0 || !a.draftOn {
		t.Fatalf("click after the switch: drawings %d draft %v", len(a.panes[0].overlays.Items()), a.draftOn)
	}

	stub := stubWithPanes(t, a, a.period)
	a.dragOn, a.pressOn = true, true
	if err := a.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	if a.draftOn || a.dragOn || a.pressOn {
		t.Fatalf("source change kept draft %v drag %v press %v", a.draftOn, a.dragOn, a.pressOn)
	}
}

func TestNewChartLoadErrorVisible(t *testing.T) {
	down := errors.New("feed down")
	failing := true
	gets := 0
	src := sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		gets++
		if failing {
			return nil, false, down
		}
		return Generate(40, symbolSeed(req.Symbol)), false, nil
	})
	a := NewChart(ChartOptions{Source: src, Indicators: []string{}})
	if !errors.Is(a.LoadError(), down) || a.Bars() != 0 {
		t.Fatalf("first load: err %v bars %d", a.LoadError(), a.Bars())
	}
	tt := ui.NewTester(a.View, 960, 720)
	if !tt.HasText("No bars") {
		t.Fatalf("empty chart texts %q", tt.Texts())
	}

	failing = false
	if err := a.SetPeriod(a.Period()); err != nil {
		t.Fatalf("retry with the same period: %v", err)
	}
	if a.LoadError() != nil || a.Bars() != 40 {
		t.Fatalf("after retry: err %v bars %d", a.LoadError(), a.Bars())
	}
	before := gets
	if err := a.SetPeriod(a.Period()); err != nil || gets != before {
		t.Fatalf("loaded period should be a no-op: err %v calls %d", err, gets-before)
	}

	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	for _, name := range []string{"ALPHA", "BETA"} {
		if err := stub.SetBars(name, hour, Generate(30, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
	}
	live := NewChart(ChartOptions{Source: &subscribeFail{StubLiveSource: stub, fail: "BETA"}, Period: hour, Indicators: []string{}})
	if live.LoadError() == nil || live.Bars() != 30 {
		t.Fatalf("subscribe failure: err %v bars %d", live.LoadError(), live.Bars())
	}
}

func TestGetBarsBufferIsCopied(t *testing.T) {
	syn := &SyntheticSource{}
	// One buffer for every GetBars: init and forward, ALPHA and BETA.
	buf := make([]Candle, 0, sampleBars)
	src := sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		bars, more, err := syn.GetBars(req)
		if err != nil {
			return nil, false, err
		}
		buf = append(buf[:0], bars...)
		return buf, more, nil
	})
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA"}, Indicators: []string{}})
	want := func(req BarRequest) []Candle {
		t.Helper()
		bars, _, err := syn.GetBars(req)
		if err != nil {
			t.Fatal(err)
		}
		return bars
	}
	init := map[string][]Candle{}
	for _, s := range a.panes {
		init[s.name] = want(BarRequest{Symbol: s.name, Period: a.period, Kind: KindInit, Limit: sampleBars})
		if !sameCandles(s.candles, init[s.name]) {
			t.Fatalf("%s init bars are another symbol's", s.name)
		}
	}
	for i := range buf {
		buf[i].Close = -1
	}
	for _, s := range a.panes {
		if !sameCandles(s.candles, init[s.name]) {
			t.Fatalf("%s bars changed when the source reused its buffer", s.name)
		}
	}

	anchor := a.panes[0].candles[0].Time
	a.fitted = true
	a.vp.Offset = 2
	a.ensureForward()
	for _, s := range a.panes {
		older := want(BarRequest{Symbol: s.name, Period: a.period, Kind: KindForward, Anchor: anchor, Limit: forwardBatch})
		if len(s.candles) != sampleBars+forwardBatch || !sameCandles(s.candles[:forwardBatch], older) {
			t.Fatalf("%s forward bars are another symbol's (len %d)", s.name, len(s.candles))
		}
	}
	if !a.panes[0].candles[forwardBatch].Time.Equal(anchor) {
		t.Fatal("prepend moved the anchor bar")
	}
}

func TestOneSidedLivePushKeepsAlignment(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	for _, name := range []string{"ALPHA", "BETA"} {
		if err := stub.SetBars(name, hour, Generate(300, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
	}
	a := NewChart(ChartOptions{Source: stub, Symbols: []string{"ALPHA", "BETA"}, Period: hour, Indicators: []string{"MA"}})
	tt := ui.NewTester(a.View, 960, 720)
	n := a.Bars()
	assertPinned := func(bars int) {
		t.Helper()
		fit := Viewport{BarWidth: a.vp.BarWidth}
		fit.FitRight(bars, a.plotW)
		if math.Abs(a.vp.Offset-fit.Offset) > 1e-6 {
			t.Fatalf("offset %v, right edge for %d bars is %v", a.vp.Offset, bars, fit.Offset)
		}
	}
	assertPinned(n)
	last := a.panes[0].candles[n-1].Time
	push := func(symbol string, at time.Time, px float64) {
		stub.Push(symbol, hour, Candle{Time: at, Open: px, High: px + 1, Low: px - 1, Close: px, Volume: 100})
	}

	push("ALPHA", last.Add(time.Hour), 120)
	tt.Frame()
	if a.Bars() != n || maLineLen(&a.panes[0]) != n {
		t.Fatalf("ALPHA-only bar was shown: bars %d ma %d", a.Bars(), maLineLen(&a.panes[0]))
	}
	assertAligned(t, a)
	assertPinned(n)
	if _, right := a.Visible(); right != n-1 {
		t.Fatalf("rightmost visible bar %d, want %d", right, n-1)
	}
	// Where ALPHA's new bar would sit, neither pane has a candle.
	price := a.painted[0].Price
	tt.Move(price.X+float32(a.vp.BarLeft(n)+a.vp.BarWidth/2), price.Y+price.H/2)
	if a.crossIndex != -1 {
		t.Fatalf("crosshair found bar %d past the shared right edge", a.crossIndex)
	}

	push("BETA", last.Add(time.Hour), 80)
	tt.Frame()
	if a.Bars() != n+1 || maLineLen(&a.panes[0]) != n+1 || maLineLen(&a.panes[1]) != n+1 {
		t.Fatalf("matched open time: bars %d", a.Bars())
	}
	assertAligned(t, a)
	assertPinned(n + 1)
	if a.panes[0].candles[n].Close != 120 || a.panes[1].candles[n].Close != 80 {
		t.Fatal("matched bars are not the pushed ones")
	}

	// BETA skips an open time ALPHA has. ALPHA's bar there can never be
	// matched, so it is dropped and the next shared open time is shown.
	push("ALPHA", last.Add(2*time.Hour), 121)
	push("ALPHA", last.Add(3*time.Hour), 122)
	if a.AlignError() != nil {
		t.Fatalf("held bars reported early: %v", a.AlignError())
	}
	push("BETA", last.Add(3*time.Hour), 82)
	if a.Bars() != n+2 || !a.panes[0].candles[n+1].Time.Equal(last.Add(3*time.Hour)) {
		t.Fatalf("after the skipped open time: bars %d", a.Bars())
	}
	assertAligned(t, a)
	if len(a.panes[0].held) != 0 || len(a.panes[1].held) != 0 {
		t.Fatal("bars stayed held")
	}
	if !errors.Is(a.AlignError(), ErrMisaligned) {
		t.Fatalf("dropped bar not reported: %v", a.AlignError())
	}
	if err := a.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil || a.AlignError() != nil {
		t.Fatalf("a load should clear AlignError: %v %v", err, a.AlignError())
	}
}

func TestLiveBarsFillEmptyChartTogether(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	a := NewChart(ChartOptions{Source: stub, Symbols: []string{"ALPHA", "BETA"}, Period: hour, Indicators: []string{}})
	if a.LoadError() != nil || a.Bars() != 0 {
		t.Fatalf("empty history: err %v bars %d", a.LoadError(), a.Bars())
	}
	at := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	stub.Push("ALPHA", hour, Candle{Time: at, Open: 10, High: 11, Low: 9, Close: 10})
	if a.Bars() != 0 {
		t.Fatal("ALPHA's first bar was shown alone")
	}
	stub.Push("BETA", hour, Candle{Time: at, Open: 20, High: 21, Low: 19, Close: 20})
	if a.Bars() != 1 {
		t.Fatalf("bars %d after both symbols opened the bar", a.Bars())
	}
	assertAligned(t, a)
}

func TestInitWindowsAlignOnOpenTime(t *testing.T) {
	alpha := Generate(601, 42)
	beta := Generate(601, 99)
	// ALPHA's feed has opened the next bar and BETA's has not, so the two
	// latest-600 windows are one bar apart.
	src := &histSource{initN: 600, hist: map[string][]Candle{"ALPHA": alpha[1:], "BETA": beta[:600]}}
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA"}, Indicators: []string{}})
	if a.LoadError() != nil || a.Bars() != 599 {
		t.Fatalf("shifted windows: err %v bars %d", a.LoadError(), a.Bars())
	}
	assertAligned(t, a)
	if !a.panes[0].candles[0].Time.Equal(alpha[1].Time) || !a.panes[0].candles[598].Time.Equal(alpha[599].Time) {
		t.Fatalf("shared range %s..%s", a.panes[0].candles[0].Time, a.panes[0].candles[598].Time)
	}
	if len(a.panes[0].held) != 1 || !a.panes[0].held[0].Time.Equal(alpha[600].Time) || len(a.panes[1].held) != 0 {
		t.Fatalf("held %d %d", len(a.panes[0].held), len(a.panes[1].held))
	}
	if !a.panes[1].more {
		t.Fatal("BETA's first bar was trimmed, so older bars exist")
	}

	gap := append(append([]Candle(nil), beta[:300]...), beta[301:600]...)
	bad := &histSource{initN: 600, hist: map[string][]Candle{"ALPHA": alpha[:600], "BETA": gap}}
	if err := a.SetSource(bad); !errors.Is(err, ErrMisaligned) {
		t.Fatalf("interior gap: %v", err)
	}
	if a.src != DataSource(src) || a.Bars() != 599 {
		t.Fatal("a refused load replaced the source or the bars")
	}
	none := &histSource{initN: 600, hist: map[string][]Candle{"ALPHA": alpha[:600]}}
	if err := a.SetSource(none); !errors.Is(err, ErrMisaligned) {
		t.Fatalf("BETA with no bars: %v", err)
	}
	c := NewChart(ChartOptions{Source: bad, Symbols: []string{"ALPHA", "BETA"}, Indicators: []string{}})
	if !errors.Is(c.LoadError(), ErrMisaligned) || c.Bars() != 0 {
		t.Fatalf("NewChart on a gap: err %v bars %d", c.LoadError(), c.Bars())
	}
}

func TestEmptySeriesCannotDragOther(t *testing.T) {
	alpha := Generate(300, 42)
	src := &histSource{initN: 300, hist: map[string][]Candle{"ALPHA": alpha}}
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA"}, Indicators: []string{}})
	if !errors.Is(a.LoadError(), ErrMisaligned) || a.Bars() != 0 || len(a.panes[0].candles) != 0 {
		t.Fatalf("BETA empty: err %v bars %d ALPHA %d", a.LoadError(), a.Bars(), len(a.panes[0].candles))
	}
	tt := ui.NewTester(a.View, 960, 720)
	box := mustBox(t, tt, "ALPHA chart")
	for i := 0; i < 4; i++ {
		tt.Press(box.X+box.W-20, box.Y+40)
		tt.Move(box.X+20, box.Y+40)
		tt.Release(box.X+20, box.Y+40)
	}
	if a.vp.Offset != -OverscrollBars {
		t.Fatalf("drag on an empty chart moved offset to %v", a.vp.Offset)
	}

	b := newApp()
	bt := ui.NewTester(b.View, 960, 720)
	n := b.Bars()
	if err := b.SetSource(src); !errors.Is(err, ErrMisaligned) {
		t.Fatalf("source with an empty BETA: %v", err)
	}
	if b.Bars() != n || len(b.panes[1].candles) != n {
		t.Fatalf("refused load emptied a pane: bars %d BETA %d", b.Bars(), len(b.panes[1].candles))
	}
	bbox := mustBox(t, bt, "ALPHA chart")
	for i := 0; i < 40; i++ {
		bt.Press(bbox.X+bbox.W-20, bbox.Y+40)
		bt.Move(bbox.X+20, bbox.Y+40)
		bt.Release(bbox.X+20, bbox.Y+40)
	}
	if maxOff := float64(n) - b.plotW/b.vp.BarWidth + OverscrollBars; b.vp.Offset > maxOff+1e-9 {
		t.Fatalf("offset %v past the shared right edge %v", b.vp.Offset, maxOff)
	}
}

func TestForwardStopsAtMisalignedOpenTime(t *testing.T) {
	alpha := Generate(300, 42)
	full := Generate(300, 99)
	beta := append(append([]Candle(nil), full[:100]...), full[101:]...)
	src := &histSource{initN: 50, hist: map[string][]Candle{"ALPHA": alpha, "BETA": beta}}
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA"}, Indicators: []string{}})
	if a.LoadError() != nil || a.Bars() != 50 {
		t.Fatalf("setup: err %v bars %d", a.LoadError(), a.Bars())
	}
	a.fitted = true
	a.vp.Offset = 2
	a.ensureForward()
	// Both batches end at bar 249, but BETA skips bar 100, so only bars
	// 101 through 249 share open times.
	if a.Bars() != 199 || a.vp.Offset != 2+149 {
		t.Fatalf("partial prepend: bars %d offset %v", a.Bars(), a.vp.Offset)
	}
	assertAligned(t, a)
	if !a.panes[0].candles[0].Time.Equal(alpha[101].Time) {
		t.Fatalf("first bar %s, want %s", a.panes[0].candles[0].Time, alpha[101].Time)
	}
	if a.AlignError() != nil || !a.panes[0].more || !a.panes[1].more {
		t.Fatalf("err %v more %v %v", a.AlignError(), a.panes[0].more, a.panes[1].more)
	}

	a.vp.Offset = 2
	a.ensureForward()
	if a.Bars() != 199 || a.vp.Offset != 2 {
		t.Fatalf("misaligned batch was prepended: bars %d offset %v", a.Bars(), a.vp.Offset)
	}
	if !errors.Is(a.AlignError(), ErrMisaligned) {
		t.Fatalf("misaligned forward not reported: %v", a.AlignError())
	}
}

func TestDuplicateSymbolsRejected(t *testing.T) {
	gets := 0
	src := sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		gets++
		return Generate(20, 1), false, nil
	})
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA", "ALPHA"}})
	if !errors.Is(a.LoadError(), ErrDuplicateSymbol) {
		t.Fatalf("LoadError %v", a.LoadError())
	}
	if gets != 0 || len(a.Symbols()) != 0 || a.Bars() != 0 {
		t.Fatalf("calls %d symbols %v bars %d", gets, a.Symbols(), a.Bars())
	}
	if _, ok := a.Bar("ALPHA", 0); ok {
		t.Fatal("a refused symbol list returned a bar")
	}
	if err := a.SetPeriod(Period{Span: 1, Unit: UnitDay}); !errors.Is(err, ErrDuplicateSymbol) {
		t.Fatalf("SetPeriod %v", err)
	}
	if err := a.SetSource(&SyntheticSource{}); !errors.Is(err, ErrDuplicateSymbol) {
		t.Fatalf("SetSource %v", err)
	}
	if gets != 0 || !errors.Is(a.LoadError(), ErrDuplicateSymbol) {
		t.Fatalf("a refused chart loaded: calls %d err %v", gets, a.LoadError())
	}
	tt := ui.NewTester(a.View, 640, 480)
	a.SetLayout(LayoutOverlay)
	tt.Frame()
	if tt.HasText("ALPHA") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// negate mirrors bars below zero, so no close is positive.
func negate(cs []Candle) []Candle {
	out := make([]Candle, len(cs))
	for i, c := range cs {
		out[i] = Candle{Time: c.Time, Open: -c.Open, High: -c.Low, Low: -c.High, Close: -c.Close, Volume: c.Volume}
	}
	return out
}

func sameScale(a, b chart.Linear) bool { return a.Lo == b.Lo && a.Hi == b.Hi }

func TestOverlayFallbackHitMatchesPaint(t *testing.T) {
	src := &histSource{initN: 200, hist: map[string][]Candle{
		"ALPHA": negate(Generate(200, 42)),
		"BETA":  Generate(200, 99),
	}}
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA"}, Layout: LayoutOverlay, Indicators: []string{}})
	tt := ui.NewTester(a.View, 1100, 1000)
	primary, compare := a.overlayPair()
	inds := a.active(&a.panes[0])
	// No bar has a positive ALPHA close, so PaintCompare draws Paint's
	// plot on MainScale.
	painted := MainScale(primary, inds, a.vp, a.plotW)
	index, raw, base := CompareScales(primary, compare, inds, a.vp, a.plotW)
	if indexable(primary, base) || !sameScale(index, painted) || !sameScale(raw, painted) {
		t.Fatalf("fallback scales index %v..%v raw %v..%v, painted %v..%v", index.Lo, index.Hi, raw.Lo, raw.Hi, painted.Lo, painted.Hi)
	}

	if err := a.SetTool(ToolHLine); err != nil {
		t.Fatal(err)
	}
	price := a.painted[0].Price
	localX, localY := float32(160), price.H*0.35
	tt.ClickAt(price.X+localX, price.Y+localY)
	items := a.panes[0].overlays.Items()
	if len(items) != 1 {
		t.Fatalf("drawings %d", len(items))
	}
	stored := items[0].Points[0].Price
	want := LocalYToPrice(float64(price.H), float64(localY), painted)
	if math.Abs(stored-want) > 1e-6 {
		t.Fatalf("stored %v, the painted scale reads %v at the click", stored, want)
	}
	if y := PriceToLocalY(float64(price.H), stored, painted); math.Abs(y-float64(localY)) > 0.5 {
		t.Fatalf("line paints at local y %v, clicked at %v", y, localY)
	}
	if _, ok := a.hit(0, float64(localX), float64(localY)); !ok {
		t.Fatal("hit test missed the line under the click")
	}
	band := ui.Rect{X: price.X, Y: price.Y + localY - 3, W: price.W, H: 6}
	if hot := countNear(tt.Image(), band, StyleFrom(nil).OverlayHot); hot < 40 {
		t.Fatalf("selected line pixels %d on the clicked row", hot)
	}
}

func TestOverlayThreeSymbolsShareHeaderBase(t *testing.T) {
	src := &histSource{initN: 200, hist: map[string][]Candle{
		"ALPHA": Generate(200, 42),
		"BETA":  Generate(200, 99),
		"GAMMA": negate(Generate(200, 7)),
	}}
	a := NewChart(ChartOptions{Source: src, Symbols: []string{"ALPHA", "BETA", "GAMMA"}, Layout: LayoutOverlay, Indicators: []string{}})
	tt := ui.NewTester(a.View, 1100, 1000)
	first, _ := a.Visible()
	primary, compare := a.overlayPair()
	_, _, painted := CompareScales(primary, compare, nil, a.vp, a.plotW)
	if painted != first || first <= 0 {
		t.Fatalf("painted base %d, first visible %d", painted, first)
	}
	if got := a.compareBase(); got != painted {
		t.Fatalf("header base %d, painted base %d", got, painted)
	}
	if math.Abs(a.indexValue(0, painted)-100) > 1e-9 || math.Abs(a.indexValue(1, painted)-100) > 1e-9 {
		t.Fatalf("header idx at the painted base %v %v", a.indexValue(0, painted), a.indexValue(1, painted))
	}
	if tt.HasText("GAMMA") {
		t.Fatalf("overlay has a header for a symbol it does not draw: %q", tt.Texts())
	}
	if r := a.Readout("GAMMA"); r == "" || strings.Contains(r, "idx") {
		t.Fatalf("GAMMA readout %q", r)
	}
	a.SetLayout(LayoutStack)
	tt.Frame()
	if !tt.HasText("GAMMA") {
		t.Fatal("stack lost the third symbol")
	}
}

func slotIndex(t *testing.T, a *Chart, name string) int {
	t.Helper()
	for i, slot := range a.slots {
		if slot.spec.Name == name {
			return i
		}
	}
	t.Fatalf("no slot %s in %v", name, a.IndicatorNames())
	return -1
}

func TestIndicatorEnabledSurvivesSync(t *testing.T) {
	a := NewChart(ChartOptions{Indicators: []string{"MA"}})
	ma, boll := a.IndicatorEnabled("MA"), a.IndicatorEnabled("BOLL")
	if ma == nil || boll == nil || !*ma || *boll {
		t.Fatal("setup: MA on, BOLL off")
	}
	maInd := a.panes[0].inds[slotIndex(t, a, "MA")]
	// Drop the last entry, as if it had registered after NewChart, so
	// SyncIndicators has to build a longer list.
	last := len(a.slots) - 1
	late := a.slots[last].spec.Name
	a.slots = a.slots[:last]
	for i := range a.panes {
		a.panes[i].inds = a.panes[i].inds[:last]
		a.panes[i].fresh = a.panes[i].fresh[:last]
	}
	a.SyncIndicators()
	if len(a.slots) != last+1 || !containsName(a.IndicatorNames(), late) || a.IndicatorOn(late) {
		t.Fatalf("sync: names %v, %s on %v", a.IndicatorNames(), late, a.IndicatorOn(late))
	}
	*ma = false
	*boll = true
	if a.IndicatorOn("MA") || !a.IndicatorOn("BOLL") {
		t.Fatalf("pointers from before the sync are detached: MA %v BOLL %v", a.IndicatorOn("MA"), a.IndicatorOn("BOLL"))
	}
	if err := a.SetIndicator("MA", true); err != nil || !*ma {
		t.Fatalf("SetIndicator after the sync did not reach the old pointer: %v %v", err, *ma)
	}
	if a.IndicatorEnabled("MA") != ma {
		t.Fatal("IndicatorEnabled returned a new pointer")
	}
	if a.panes[0].inds[slotIndex(t, a, "MA")] != maInd {
		t.Fatal("sync replaced a calculated indicator")
	}
}

// calcCounter counts Calc on the indicator it wraps.
type calcCounter struct {
	Indicator
	calls int
	n     int
}

func (c *calcCounter) Calc(cs []Candle) {
	c.calls++
	c.n = len(cs)
	c.Indicator.Calc(cs)
}

func TestOffIndicatorNotCalculatedUntilEnabled(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	for _, name := range []string{"ALPHA", "BETA"} {
		if err := stub.SetBars(name, hour, Generate(800, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
	}
	a := NewChart(ChartOptions{Source: stub, Symbols: []string{"ALPHA", "BETA"}, Period: hour, Indicators: []string{"MA"}})
	j := slotIndex(t, a, "BOLL")
	counters := make([]*calcCounter, len(a.panes))
	for i := range a.panes {
		counters[i] = &calcCounter{Indicator: a.panes[i].inds[j]}
		a.panes[i].inds[j] = counters[i]
	}
	calls := func(want int) {
		t.Helper()
		for i, c := range counters {
			if c.calls != want {
				t.Fatalf("%s BOLL Calc %d times, want %d", a.panes[i].name, c.calls, want)
			}
		}
	}
	tt := ui.NewTester(a.View, 960, 720)
	pushBoth(stub, a, 1)
	a.vp.Offset = 2
	tt.Frame()
	if a.Bars() != sampleBars+1+forwardBatch {
		t.Fatalf("setup: live bar and forward load, bars %d", a.Bars())
	}
	a.SyncIndicators()
	tt.Frame()
	calls(0)

	*a.IndicatorEnabled("BOLL") = true
	tt.Frame()
	calls(1)
	for i, c := range counters {
		if c.n != len(a.panes[i].candles) {
			t.Fatalf("BOLL calculated %d bars, pane has %d", c.n, len(a.panes[i].candles))
		}
	}
	tt.Frame()
	calls(1)
	pushBoth(stub, a, 1)
	calls(2)

	if err := a.SetIndicator("BOLL", false); err != nil {
		t.Fatal(err)
	}
	pushBoth(stub, a, 1)
	tt.Frame()
	calls(2)
	if err := a.SetIndicator("BOLL", true); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	calls(3)
	if err := a.SetIndicators([]string{"MA"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetIndicators([]string{"MA", "BOLL"}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	calls(3)
}
