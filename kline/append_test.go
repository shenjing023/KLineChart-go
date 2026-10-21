package kline

import (
	"errors"
	"testing"
	"time"
)

// UpdateLast on a bar that is still held for the other symbols revises
// the held copy and leaves the indicators alone.
func TestUpdateLastHeldBar(t *testing.T) {
	alice := Generate(30, 1)
	bob := Generate(30, 2)
	c, _ := replayChart(t, []string{"600519", "000001"}, map[string][]Candle{"600519": alice, "000001": bob}, LayoutStack)
	n := c.Bars()
	next := alice[len(alice)-1]
	next.Time = next.Time.Add(time.Hour)
	next.Close = 150
	if err := c.AppendBar("600519", next); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != n || len(c.panes[0].held) != 1 {
		t.Fatalf("600519's bar should be held: bars %d held %d", c.Bars(), len(c.panes[0].held))
	}
	ma := c.panes[0].inds[0].(*MA)
	lines := ma.Lines()[0][n-1]

	revised := next
	revised.Close = 151
	revised.Time = time.Time{} // a zero time keeps the held open
	if err := c.UpdateLast("600519", revised); err != nil {
		t.Fatal(err)
	}
	if got := c.panes[0].held[0].Close; got != 151 {
		t.Fatalf("held close %v, want 151", got)
	}
	if !c.panes[0].held[0].Time.Equal(next.Time) {
		t.Fatalf("zero time did not keep the open: %v", c.panes[0].held[0].Time)
	}
	if c.Bars() != n {
		t.Fatalf("a held revision changed the bar count: %d", c.Bars())
	}
	if got := ma.Lines()[0][n-1]; got != lines {
		t.Fatalf("a held revision touched the indicators: %v -> %v", lines, got)
	}

	// A held bar with the wrong open time is refused.
	wrong := next
	wrong.Time = next.Time.Add(time.Hour)
	if err := c.UpdateLast("600519", wrong); !errors.Is(err, ErrBarTime) {
		t.Fatalf("held wrong open: %v", err)
	}
	// An unknown symbol is refused before anything else.
	if err := c.UpdateLast("NOPE", next); !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("unknown symbol: %v", err)
	}
	// When every symbol has the bar it is shown, and then a revision
	// updates the indicator tail.
	if err := c.AppendBar("000001", next); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != n+1 {
		t.Fatalf("bars %d, want %d", c.Bars(), n+1)
	}
	shown := c.panes[0].candles[n]
	shown.Close = 160
	if err := c.UpdateLast("600519", shown); err != nil {
		t.Fatal(err)
	}
	if c.panes[0].candles[n].Close != 160 {
		t.Fatal("shown revision did not land")
	}
	if err := c.UpdateLast("600519", Candle{Time: shown.Time.Add(-time.Hour)}); !errors.Is(err, ErrBarTime) {
		t.Fatalf("shown wrong open: %v", err)
	}
}

// UpdateLast on an empty series reports ErrBarTime instead of panicking.
func TestUpdateLastEmptySeries(t *testing.T) {
	src := sourceFunc(func(BarRequest) ([]Candle, bool, error) { return nil, false, nil })
	c := NewChart(ChartOptions{Source: src, Symbols: []string{"600519"}, Indicators: []string{}})
	if c.Bars() != 0 {
		t.Fatalf("setup: bars %d", c.Bars())
	}
	if err := c.UpdateLast("600519", Candle{Close: 1}); !errors.Is(err, ErrBarTime) {
		t.Fatalf("empty series: %v", err)
	}
	// AppendBar treats an empty chart as already at the edge.
	at := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	if err := c.AppendBar("600519", Candle{Time: at, Open: 1, High: 2, Low: 0.5, Close: 1.5}); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != 1 {
		t.Fatalf("first bar: %d", c.Bars())
	}
	// A zero open time is refused.
	if err := c.AppendBar("600519", Candle{}); !errors.Is(err, ErrBarTime) {
		t.Fatalf("zero time: %v", err)
	}
}
