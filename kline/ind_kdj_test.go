package kline

import (
	"math"
	"testing"
)

// kdjBars is long enough for KDJ(9, 3, 3). K, D, and J start at index 8.
// The first seed is 50, which is KLineChart's stand-in for a missing
// previous K or D. A flat bar (high == low) uses a denominator of 1.
func kdjBars() []Candle {
	rows := [][3]float64{
		{10, 8, 9},
		{11, 9, 10},
		{12, 8, 11},
		{13, 10, 12},
		{12, 9, 10},
		{14, 11, 13},
		{15, 12, 14},
		{14, 10, 11},
		{16, 12, 15},
		{17, 13, 16},
		{16, 14, 15},
		{18, 15, 17},
	}
	cs := make([]Candle, len(rows))
	for i, row := range rows {
		cs[i] = Candle{High: row[0], Low: row[1], Close: row[2], Open: row[2]}
	}
	return cs
}

func TestKDJFixture(t *testing.T) {
	cs := kdjBars()
	k := NewKDJ(9, 3, 3)
	if k.Name() != "KDJ" || k.PaneKind() != PaneSub {
		t.Fatalf("name %q kind %q", k.Name(), k.PaneKind())
	}
	k.Calc(cs)
	if len(k.K) != len(cs) || len(k.D) != len(cs) || len(k.J) != len(cs) {
		t.Fatalf("lengths K %d D %d J %d", len(k.K), len(k.D), len(k.J))
	}
	for i := 0; i < 8; i++ {
		if !math.IsNaN(k.K[i]) || !math.IsNaN(k.D[i]) || !math.IsNaN(k.J[i]) {
			t.Fatalf("index %d should be warming up", i)
		}
	}
	assertClose(t, "K[8]", k.K[8], 62.5)
	assertClose(t, "D[8]", k.D[8], 54.166666666666664)
	assertClose(t, "J[8]", k.J[8], 79.16666666666667)
	assertClose(t, "K[11]", k.K[11], 78.60082304526749)
	assertClose(t, "D[11]", k.D[11], 69.1358024691358)
	assertClose(t, "J[11]", k.J[11], 97.53086419753089)
	for i := 8; i < len(cs); i++ {
		want := 3*k.K[i] - 2*k.D[i]
		if math.Abs(k.J[i]-want) > 1e-9 {
			t.Fatalf("J[%d] = %v, want 3K-2D %v", i, k.J[i], want)
		}
	}
	if k.ValueAt(7) != "" || k.ValueAt(-1) != "" {
		t.Fatal("ValueAt should be empty before K exists")
	}
	got := k.ValueAt(8)
	want := "K " + FormatPrice(k.K[8]) + "  D " + FormatPrice(k.D[8]) + "  J " + FormatPrice(k.J[8])
	if got != want {
		t.Fatalf("ValueAt %q, want %q", got, want)
	}
	lo, hi := k.YDomain(0, len(cs)-1)
	if lo >= 0 || hi <= 100 {
		t.Fatalf("KDJ domain should cover 0–100: %v %v", lo, hi)
	}
	k.J[11] = 140
	_, hi = k.YDomain(0, len(cs)-1)
	if hi <= 140 {
		t.Fatalf("a J above 100 should widen the scale, hi %v", hi)
	}
}

func TestKDJFlatRangeUsesUnitDenominator(t *testing.T) {
	cs := make([]Candle, 10)
	for i := range cs {
		cs[i] = Candle{Open: 5, High: 5, Low: 5, Close: 5}
	}
	k := NewKDJ(9, 3, 3)
	k.Calc(cs)
	// RSV is 0 because close == low and high == low, so the denominator is 1.
	// K = (2*50 + 0) / 3.
	assertClose(t, "K[8]", k.K[8], 100.0/3)
	assertClose(t, "D[8]", k.D[8], (2*50+100.0/3)/3)
	assertClose(t, "J[8]", k.J[8], 3*k.K[8]-2*k.D[8])
}
