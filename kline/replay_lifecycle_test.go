package kline

import (
	"errors"
	"testing"
	"time"
)

// A refused SetSource must not end the replay.
func TestSetSourceNilKeepsReplay(t *testing.T) {
	base := Generate(40, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(10); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSource(nil); !errors.Is(err, ErrNilSource) {
		t.Fatalf("SetSource(nil) = %v, want ErrNilSource", err)
	}
	if c.replay != replay {
		t.Fatal("a refused SetSource detached the replay")
	}
	if c.Bars() != 10 {
		t.Fatalf("bars %d, want the revealed 10", c.Bars())
	}
	if err := replay.Step(1); err != nil {
		t.Fatalf("the replay cannot resume: %v", err)
	}
	if c.Bars() != 11 {
		t.Fatalf("bars after step %d", c.Bars())
	}
}

// A SetSource whose load fails must not end the replay either.
func TestSetSourceFailureKeepsReplay(t *testing.T) {
	base := Generate(40, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(12); err != nil {
		t.Fatal(err)
	}
	down := errors.New("feed down")
	bad := sourceFunc(func(BarRequest) ([]Candle, bool, error) { return nil, false, down })
	if err := c.SetSource(bad); !errors.Is(err, down) {
		t.Fatalf("SetSource(down) = %v, want the source error", err)
	}
	if c.replay != replay {
		t.Fatal("a failed SetSource detached the replay")
	}
	if c.Bars() != 12 {
		t.Fatalf("bars %d", c.Bars())
	}
	if err := replay.Step(1); err != nil {
		t.Fatalf("the replay cannot resume: %v", err)
	}
}

// A successful SetSource still ends the replay, as documented.
func TestSetSourceSuccessEndsReplay(t *testing.T) {
	base := Generate(40, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(10); err != nil {
		t.Fatal(err)
	}
	replacement := &histSource{initN: 40, hist: map[string][]Candle{"600519": Generate(40, 9)}}
	want := replacement.hist["600519"]
	if err := c.SetSource(replacement); err != nil {
		t.Fatal(err)
	}
	if c.replay != nil {
		t.Fatal("a successful SetSource kept the replay")
	}
	if err := replay.Step(1); !errors.Is(err, ErrReplayClosed) {
		t.Fatalf("Step after SetSource = %v, want ErrReplayClosed", err)
	}
	sameBars(t, c.panes[0].candles, want)
}

// Closing a replay must not disable paging left.
func TestReplayCloseRestoresMore(t *testing.T) {
	all := Generate(900, 7)
	src := &countSource{keepHandlers: &keepHandlers{StubLiveSource: &StubLiveSource{}}}
	hour := Period{Span: 1, Unit: UnitHour}
	if err := src.SetBars("600519", hour, all); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"600519"},
		Period:     hour,
		Indicators: []string{},
	})
	// The init window is sampleBars, so older bars exist.
	if c.Bars() != sampleBars || !c.panes[0].more {
		t.Fatalf("setup: bars %d more %v", c.Bars(), c.panes[0].more)
	}
	replay, err := StartReplay(c, map[string][]Candle{"600519": all})
	if err != nil {
		t.Fatal(err)
	}
	if c.panes[0].more {
		t.Fatal("a replay should clear more")
	}
	if err := replay.SeekIndex(500); err != nil {
		t.Fatal(err)
	}
	replay.Close()
	if !c.panes[0].more {
		t.Fatal("Close left more false, so paging left is dead")
	}
	c.fitted = true
	c.vp.Offset = 2
	gets := src.gets
	c.ensureForward()
	if src.gets == gets {
		t.Fatal("a forward load was not attempted after the replay closed")
	}
}

// Close is not final: a later SetPeriod loads and resubscribes again.
// The README documents that; this test pins it.
func TestCloseThenSetPeriodResubscribes(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	four := Period{Span: 4, Unit: UnitHour}
	stub := &keepHandlers{StubLiveSource: &StubLiveSource{}}
	for _, name := range []string{"600519", "000001"} {
		if err := stub.SetBars(name, hour, Generate(60, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
		if err := stub.SetBars(name, four, sampleWalk(60, symbolSeed(name), otherOrigin, 4*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	c := NewChart(ChartOptions{Source: stub, Symbols: []string{"600519", "000001"}, Period: hour, Indicators: []string{}})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	before := len(stub.handlers)
	if err := c.SetPeriod(four); err != nil {
		t.Fatal(err)
	}
	if len(stub.handlers) <= before {
		t.Fatal("SetPeriod after Close did not resubscribe")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseThenSamePeriodReopensLive(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	all := Generate(2, 1)
	stub := &StubLiveSource{}
	if err := stub.SetBars("600519", hour, all[:1]); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"600519"},
		Period:     hour,
		Indicators: []string{},
	})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	subs := len(stub.subs)
	if err := c.SetPeriod(c.Period()); err != nil {
		t.Fatal(err)
	}
	if len(stub.subs) <= subs {
		t.Fatal("same-period reopen did not resubscribe")
	}
	stub.Push("600519", hour, all[1])
	if c.Bars() != 2 {
		t.Fatalf("bars after reopen and push: %d", c.Bars())
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseAfterReopenAgainStopsUpdates(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	all := Generate(3, 2)
	stub := &StubLiveSource{}
	if err := stub.SetBars("600519", hour, all[:1]); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"600519"},
		Period:     hour,
		Indicators: []string{},
	})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	stub.Push("600519", hour, all[1])
	if c.Bars() != 1 {
		t.Fatalf("a push after the second Close changed bars to %d", c.Bars())
	}
}

func TestSetPeriodFailureKeepsClosedState(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	bad := sourceFunc(func(BarRequest) ([]Candle, bool, error) {
		return nil, false, errors.New("feed down")
	})
	c := NewChart(ChartOptions{
		Source:     bad,
		Symbols:    []string{"600519"},
		Period:     hour,
		Indicators: []string{},
	})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPeriod(c.Period()); err == nil {
		t.Fatal("same-period reopen ignored the load failure")
	}
	if !c.closed {
		t.Fatal("a failed reopen cleared the Chart closed state")
	}
}

// Close is not final for a replay either: the revealed bars are a prefix,
// not the loaded window, so a same-period SetPeriod must load the suffix
// again rather than take the no-op path.
func TestReplayCloseThenSamePeriodReloads(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	all := Generate(900, 7)
	src := newCountSource(t, hour, 0)
	if err := src.SetBars("600519", hour, all); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"600519"},
		Period:     hour,
		Indicators: []string{},
	})
	replay, err := StartReplay(c, map[string][]Candle{"600519": all})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(48); err != nil {
		t.Fatal(err)
	}
	replay.Close()
	if c.Bars() != 48 {
		t.Fatalf("Close changed the revealed bars to %d", c.Bars())
	}
	gets := src.gets
	if err := c.SetPeriod(c.Period()); err != nil {
		t.Fatal(err)
	}
	if src.gets == gets {
		t.Fatal("same-period SetPeriod after Replay.Close did not ask the source")
	}
	if c.Bars() != sampleBars {
		t.Fatalf("bars after the reload %d, want the loaded window %d", c.Bars(), sampleBars)
	}
}

// A replay brings its own bars, so it clears a LoadError left by a failed
// first load. A refused StartReplay leaves that error in place.
func TestReplayClearsLoadError(t *testing.T) {
	down := errors.New("feed down")
	bad := sourceFunc(func(BarRequest) ([]Candle, bool, error) { return nil, false, down })
	c := NewChart(ChartOptions{Source: bad, Symbols: []string{"600519", "000001"}, Indicators: []string{}})
	if !errors.Is(c.LoadError(), down) {
		t.Fatalf("setup load error %v", c.LoadError())
	}
	// A missing symbol refuses the replay before it touches the chart.
	refused, err := StartReplay(c, map[string][]Candle{"600519": Generate(5, 1)})
	if !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("StartReplay with a missing symbol: %v", err)
	}
	if refused != nil || !errors.Is(c.LoadError(), down) {
		t.Fatal("a refused StartReplay cleared the load error")
	}
	replay, err := StartReplay(c, map[string][]Candle{"600519": Generate(10, 1), "000001": Generate(10, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if c.LoadError() != nil {
		t.Fatalf("StartReplay left load error %v", c.LoadError())
	}
	if err := replay.Step(1); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != 1 || c.LoadError() != nil {
		t.Fatalf("bars %d load error %v", c.Bars(), c.LoadError())
	}
}
