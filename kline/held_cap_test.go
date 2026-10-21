package kline

import (
	"errors"
	"testing"
	"time"
)

// A symbol whose feed stops must not grow held without a bound. The
// chart keeps a window of pending bars and reports the drop through
// AlignError so a host can see that the symbols no longer line up.
func TestHeldIsCapped(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	for _, name := range []string{"600519", "000001"} {
		if err := stub.SetBars(name, hour, Generate(200, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
	}
	c := NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"600519", "000001"},
		Period:     hour,
		Indicators: []string{},
	})
	if c.LoadError() != nil || c.Bars() != 200 {
		t.Fatalf("setup: err %v bars %d", c.LoadError(), c.Bars())
	}
	start := c.panes[0].candles[199].Time
	const pushes = 20000
	for i := 1; i <= pushes; i++ {
		stub.Push("600519", hour, Candle{
			Time: start.Add(time.Duration(i) * time.Hour),
			Open: 100, High: 101, Low: 99, Close: 100, Volume: 1,
		})
	}
	if c.Bars() != 200 {
		t.Fatalf("a one-sided feed changed the bar count: %d", c.Bars())
	}
	if held := len(c.panes[0].held); held > maxHeldBars {
		t.Fatalf("600519 holds %d pending bars, cap is %d", held, maxHeldBars)
	}
	if held := len(c.panes[0].held); held == 0 {
		t.Fatal("the newest pending bars should be kept, not discarded")
	}
	if !errors.Is(c.AlignError(), ErrMisaligned) {
		t.Fatalf("dropped pending bars were not reported: %v", c.AlignError())
	}
	// The kept window holds the newest open times, so 000001 catching up
	// at the newest one still appends both.
	newest := start.Add(pushes * time.Hour)
	if !c.panes[0].held[len(c.panes[0].held)-1].Time.Equal(newest) {
		t.Fatalf("newest pending bar is %s, want %s", c.panes[0].held[len(c.panes[0].held)-1].Time, newest)
	}
	stub.Push("000001", hour, Candle{
		Time: newest,
		Open: 50, High: 51, Low: 49, Close: 50, Volume: 1,
	})
	if c.Bars() != 201 {
		t.Fatalf("matched push after the cap did not append: bars %d", c.Bars())
	}
	if !c.panes[0].candles[200].Time.Equal(newest) || !c.panes[1].candles[200].Time.Equal(newest) {
		t.Fatal("the appended bar is not the matched open time")
	}
	// A load clears the report.
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	if c.AlignError() != nil {
		t.Fatalf("a load should clear AlignError: %v", c.AlignError())
	}
}

// The cap applies to the manual append path too, so a host that pushes
// one symbol ahead cannot grow it without a bound.
func TestHeldCapOnAppendBar(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	for _, name := range []string{"600519", "000001"} {
		if err := stub.SetBars(name, hour, Generate(40, symbolSeed(name))); err != nil {
			t.Fatal(err)
		}
	}
	c := NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"600519", "000001"},
		Period:     hour,
		Indicators: []string{},
	})
	start := c.panes[0].candles[len(c.panes[0].candles)-1].Time
	for i := 1; i <= maxHeldBars*3; i++ {
		bar := Candle{Time: start.Add(time.Duration(i) * time.Hour), Open: 1, High: 2, Low: 0.5, Close: 1.5}
		if err := c.AppendBar("600519", bar); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if held := len(c.panes[0].held); held > maxHeldBars {
		t.Fatalf("AppendBar left %d pending bars, cap is %d", held, maxHeldBars)
	}
	if len(c.panes[1].held) != 0 {
		t.Fatalf("000001 should have nothing pending: %d", len(c.panes[1].held))
	}
	if !errors.Is(c.AlignError(), ErrMisaligned) {
		t.Fatalf("AppendBar past the cap was not reported: %v", c.AlignError())
	}
}

func TestInitialHeldBarsUseCap(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	bars := Generate(600, 42)
	feed := &StubLiveSource{}
	if err := feed.SetBars("A", hour, bars); err != nil {
		t.Fatal(err)
	}
	if err := feed.SetBars("B", hour, bars[:1]); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     feed,
		Symbols:    []string{"A", "B"},
		Period:     hour,
		Indicators: []string{},
	})
	defer c.Close()
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
	if got := len(c.panes[0].held); got > maxHeldBars {
		t.Fatalf("initial load holds %d pending bars, cap is %d", got, maxHeldBars)
	}
	if got := len(c.panes[0].held); got == 0 {
		t.Fatal("initial load discarded the newest pending window")
	}
	if !errors.Is(c.AlignError(), ErrMisaligned) {
		t.Fatalf("initial pending drop was not reported: %v", c.AlignError())
	}
}

func TestSetPeriodFailureKeepsInitialPendingReport(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	bars := Generate(600, 42)
	badFour := errors.New("four-hour feed down")
	feed := &reloadFailSource{
		hour: bars,
		one:  bars[:1],
		err:  badFour,
	}
	c := NewChart(ChartOptions{
		Source:     feed,
		Symbols:    []string{"A", "B"},
		Period:     hour,
		Indicators: []string{},
	})
	defer c.Close()
	beforeHeld := len(c.panes[0].held)
	beforeBars := c.Bars()
	beforePeriod := c.Period()
	wantAlign := c.AlignError()
	if !errors.Is(wantAlign, ErrMisaligned) || beforeHeld > maxHeldBars {
		t.Fatalf("setup held %d align %v", beforeHeld, wantAlign)
	}
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); !errors.Is(err, badFour) {
		t.Fatalf("SetPeriod = %v, want the feed error", err)
	}
	if c.Bars() != beforeBars || c.Period() != beforePeriod || len(c.panes[0].held) != beforeHeld || c.AlignError() != wantAlign {
		t.Fatalf("failed reload changed bars %d period %s held %d align %v", c.Bars(), c.Period(), len(c.panes[0].held), c.AlignError())
	}
}

type reloadFailSource struct {
	hour, one []Candle
	err       error
}

func (s *reloadFailSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if req.Period.Span == 4 {
		return nil, false, s.err
	}
	if req.Symbol == "B" {
		return s.one, false, nil
	}
	return s.hour, false, nil
}

func TestInitialHeldDropSurvivesInstallAndClearsOnReload(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	bars := Generate(600, 42)
	feed := &StubLiveSource{}
	if err := feed.SetBars("A", hour, bars); err != nil {
		t.Fatal(err)
	}
	if err := feed.SetBars("B", hour, bars[:1]); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     feed,
		Symbols:    []string{"A", "B"},
		Period:     hour,
		Indicators: []string{},
	})
	defer c.Close()
	if err := c.LoadError(); err != nil || !errors.Is(c.AlignError(), ErrMisaligned) {
		t.Fatalf("setup load %v align %v", err, c.AlignError())
	}
	// Make the next reload fully aligned. It must clear the previous
	// pending-drop report rather than carrying it across loads.
	if err := feed.SetBars("B", hour, bars); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSource(feed); err != nil {
		t.Fatal(err)
	}
	if len(c.panes[0].held) != 0 || c.AlignError() != nil {
		t.Fatalf("aligned reload kept held %d report %v", len(c.panes[0].held), c.AlignError())
	}
}
