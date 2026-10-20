package kline

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
)

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
