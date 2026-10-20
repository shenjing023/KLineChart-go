package kline

import (
	"math"
	"testing"
)

func TestMACalcMatchesSMA(t *testing.T) {
	cs := []Candle{
		{Close: 1}, {Close: 2}, {Close: 3}, {Close: 4}, {Close: 5},
		{Close: 6}, {Close: 7}, {Close: 8},
	}
	ma := NewMA(5, 20)
	if ma.Name() != "MA" || ma.PaneKind() != PaneMain {
		t.Fatalf("name %q kind %q", ma.Name(), ma.PaneKind())
	}
	ma.Calc(cs)
	lines := ma.Lines()
	if len(lines) != 2 {
		t.Fatalf("lines %d", len(lines))
	}
	for pi, period := range []int{5, 20} {
		want := SMA(cs, period)
		if len(lines[pi]) != len(want) {
			t.Fatalf("period %d len %d, want %d", period, len(lines[pi]), len(want))
		}
		for i := range want {
			if math.IsNaN(want[i]) != math.IsNaN(lines[pi][i]) || (!math.IsNaN(want[i]) && math.Abs(lines[pi][i]-want[i]) > 1e-9) {
				t.Fatalf("MA%d[%d] = %v, SMA %v", period, i, lines[pi][i], want[i])
			}
		}
	}
	// Index 4 is the first full MA5 window. MA20 is still warming up.
	got := ma.ValueAt(4)
	want := "MA5 " + FormatPrice(SMA(cs, 5)[4])
	if got != want {
		t.Fatalf("ValueAt %q, want %q", got, want)
	}
	if ma.ValueAt(-1) != "" || ma.ValueAt(len(cs)) != "" {
		t.Fatal("ValueAt should be empty outside the series")
	}
}

func TestRegistryListsMAAndMACD(t *testing.T) {
	got := map[string]string{}
	for _, spec := range Registry() {
		ind := spec.New()
		got[spec.Name] = spec.Kind
		if ind.Name() != spec.Name || ind.PaneKind() != spec.Kind {
			t.Fatalf("spec %+v built %s/%s", spec, ind.Name(), ind.PaneKind())
		}
	}
	if got["MA"] != PaneMain || got["MACD"] != PaneSub {
		t.Fatalf("registry %v", got)
	}
}
