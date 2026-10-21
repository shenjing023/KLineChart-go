package kline

import (
	"errors"
	"testing"
	"time"
)

// A series that is not in ascending open-time order would be aggregated,
// searched, and drawn as if it were, so a load must refuse it instead of
// painting the wrong bars.
func TestLoadRejectsUnsortedBars(t *testing.T) {
	shuffled := Generate(20, 3)
	shuffled[5], shuffled[15] = shuffled[15], shuffled[5]
	src := sourceFunc(func(BarRequest) ([]Candle, bool, error) { return shuffled, false, nil })

	c := NewChart(ChartOptions{Source: src, Symbols: []string{"600519"}, Indicators: []string{}})
	if !errors.Is(c.LoadError(), ErrBarOrder) {
		t.Fatalf("LoadError = %v, want ErrBarOrder", c.LoadError())
	}
	if c.Bars() != 0 {
		t.Fatalf("an unsorted load installed %d bars", c.Bars())
	}

	// SetSource and SetPeriod report it the same way and keep the bars.
	good := NewChart(ChartOptions{
		Source:  sourceFunc(func(BarRequest) ([]Candle, bool, error) { return Generate(30, 4), false, nil }),
		Symbols: []string{"600519"}, Indicators: []string{},
	})
	if good.LoadError() != nil || good.Bars() != 30 {
		t.Fatalf("setup: err %v bars %d", good.LoadError(), good.Bars())
	}
	if err := good.SetSource(src); !errors.Is(err, ErrBarOrder) {
		t.Fatalf("SetSource = %v, want ErrBarOrder", err)
	}
	if good.Bars() != 30 {
		t.Fatalf("a refused source changed the bars to %d", good.Bars())
	}
	if err := good.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatalf("a good period after the refusal: %v", err)
	}
}

// A duplicate open time is not ascending either: the second bar would
// overwrite the first on every lookup that binary-searches the series.
func TestLoadRejectsDuplicateOpenTime(t *testing.T) {
	cs := Generate(10, 5)
	cs[6].Time = cs[5].Time
	src := sourceFunc(func(BarRequest) ([]Candle, bool, error) { return cs, false, nil })
	c := NewChart(ChartOptions{Source: src, Symbols: []string{"600519"}, Indicators: []string{}})
	if !errors.Is(c.LoadError(), ErrBarOrder) {
		t.Fatalf("duplicate open time: %v", c.LoadError())
	}
}

// StartReplay keeps the full series private, so it has to validate the
// same way; a host can hand it any slice.
func TestStartReplayRejectsUnsortedBars(t *testing.T) {
	shuffled := Generate(20, 3)
	shuffled[5], shuffled[15] = shuffled[15], shuffled[5]
	c := NewChart(ChartOptions{Symbols: []string{"600519"}, Indicators: []string{}})
	r, err := StartReplay(c, map[string][]Candle{"600519": shuffled})
	if !errors.Is(err, ErrBarOrder) {
		t.Fatalf("StartReplay = %v, want ErrBarOrder", err)
	}
	if r != nil || c.replay != nil {
		t.Fatal("a refused replay was attached")
	}
	// A sorted series still starts.
	if r, err := StartReplay(c, map[string][]Candle{"600519": Generate(20, 3)}); err != nil || r == nil {
		t.Fatalf("sorted StartReplay: %v %v", r, err)
	}
}

// Live bars are already checked against the newest open time; a
// revision of an older bar is ignored rather than inserted.
func TestLiveBarOrderIsStillChecked(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	if err := stub.SetBars("600519", hour, Generate(20, 1)); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{Source: stub, Symbols: []string{"600519"}, Period: hour, Indicators: []string{}})
	last := c.panes[0].candles[19].Time
	stub.Push("600519", hour, Candle{Time: last.Add(-5 * time.Hour), Close: 1})
	if c.Bars() != 20 {
		t.Fatalf("an older live bar was inserted: bars %d", c.Bars())
	}
	if c.AlignError() != nil {
		t.Fatalf("an ignored older bar should not be an alignment error: %v", c.AlignError())
	}
}
